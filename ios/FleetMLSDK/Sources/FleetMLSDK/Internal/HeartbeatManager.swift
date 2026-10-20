import Foundation
import GRPC

/// Thrown by DeviceAPI implementations when the server rejects the token.
struct UnauthenticatedError: Error {}

/// The authenticated device RPCs the heartbeat loop uses.
protocol DeviceAPI {
    func heartbeat(token: String, request: Fleetml_Device_V1_HeartbeatRequest) async throws -> Fleetml_Device_V1_HeartbeatResponse
    func refreshToken(token: String) async throws -> Fleetml_Device_V1_RefreshTokenResponse
}

/// The periodic heartbeat: refresh the token when due, report active models,
/// apply assignments and experiment arms; a rejected token triggers re-registration.
final class HeartbeatManager {
    private let api: DeviceAPI
    private let tokens: TokenStore
    private let models: ModelManager
    private let telemetry: TelemetryReporter?
    private let delegate: () -> FleetMLDelegate?
    private let reRegister: () async -> Void
    private let onConfig: (Fleetml_Device_V1_DeviceConfig) -> Void
    private let now: () -> Date
    private let lock = NSLock()
    private var loop: Task<Void, Never>?
    private var connected: Bool?
    private(set) var interval: TimeInterval

    init(api: DeviceAPI, tokens: TokenStore, models: ModelManager, telemetry: TelemetryReporter?,
         delegate: @escaping () -> FleetMLDelegate?, interval: TimeInterval,
         reRegister: @escaping () async -> Void = {}, onConfig: @escaping (Fleetml_Device_V1_DeviceConfig) -> Void = { _ in },
         now: @escaping () -> Date = Date.init) {
        self.api = api
        self.tokens = tokens
        self.models = models
        self.telemetry = telemetry
        self.delegate = delegate
        self.interval = max(1, interval)
        self.reRegister = reRegister
        self.onConfig = onConfig
        self.now = now
    }

    /// Starts the loop unless it is running: at launch both the start task and
    /// the foreground notification start it, and restarting would cancel the
    /// first heartbeat in flight.
    func start() {
        lock.withLock {
            guard loop == nil else { return }
            loop = Task { [weak self] in
                while !Task.isCancelled {
                    guard let self else { return }
                    _ = await self.beatOnce()
                    try? await Task.sleep(nanoseconds: UInt64(self.lock.withLock { self.interval } * 1_000_000_000))
                }
            }
        }
    }

    func stop() {
        lock.withLock {
            loop?.cancel()
            loop = nil
        }
    }

    func setInterval(_ seconds: TimeInterval) {
        if seconds > 0 { lock.withLock { interval = seconds } }
    }

    /// One heartbeat; true if the server was reached.
    @discardableResult
    func beatOnce() async -> Bool {
        guard let deviceID = tokens.deviceID, var token = tokens.token else { return false }
        do {
            if TokenRefreshPolicy.needsRefresh(now: now(), expiresAt: tokens.tokenExpiresAt, issuedAt: tokens.tokenIssuedAt) {
                do {
                    let r = try await api.refreshToken(token: token)
                    tokens.saveToken(r.authToken, expiresAtUnix: r.tokenExpiresAt)
                    token = r.authToken
                    Log.sdk.info("Device token refreshed")
                } catch is UnauthenticatedError {
                    throw UnauthenticatedError()
                } catch {
                    Log.sdk.warning("Token refresh failed: \(error.localizedDescription, privacy: .public)") // retried next heartbeat
                }
            }
            var request = Fleetml_Device_V1_HeartbeatRequest()
            request.deviceID = deviceID
            request.loadedModels = models.loadedStatuses
            let response = try await api.heartbeat(token: token, request: request)
            Log.sdk.debug("Heartbeat: \(response.assignments.count, privacy: .public) assignment(s), next in \(response.nextHeartbeatSeconds, privacy: .public)s")
            setConnected(true)
            setInterval(TimeInterval(response.nextHeartbeatSeconds))
            if response.hasConfig { onConfig(response.config) }
            telemetry?.setExperimentArms(Dictionary(response.experiments.map { ($0.modelVersionID, $0.armID) }, uniquingKeysWith: { a, _ in a }))
            // Keys first: an assignment may be signed by a key the device has not seen.
            models.setSigningKeys(response.signingKeys)
            await models.apply(response.assignments)
            return true
        } catch is UnauthenticatedError {
            Log.sdk.warning("Token rejected by server, re-registering")
            await reRegister()
            setConnected(false)
            return false
        } catch where Self.isCancellation(error) {
            // stop() (the app went to the background) cancelled the call:
            // nothing failed, and the connection state is unknown, not lost.
            Log.sdk.debug("Heartbeat cancelled")
            return false
        } catch {
            Log.sdk.warning("Heartbeat failed: \(error.localizedDescription, privacy: .public)")
            setConnected(false)
            return false
        }
    }

    static func isCancellation(_ error: Error) -> Bool {
        if error is CancellationError { return true }
        if let status = error as? GRPCStatus { return status.code == .cancelled }
        return false
    }

    private func setConnected(_ value: Bool) {
        let changed = lock.withLock { () -> Bool in
            if connected == value { return false }
            connected = value
            return true
        }
        guard changed else { return }
        delegate()?.fleetML(connectionChanged: value)
        if value { telemetry?.onReconnected() }
    }
}
