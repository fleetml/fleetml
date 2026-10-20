import GRPC
import XCTest
@testable import FleetMLSDK

final class TokenRefreshPolicyTests: XCTestCase {
    func testLongTokensRefreshSixDaysBeforeExpiry() {
        let issued = Date()
        let expires = issued.addingTimeInterval(30 * 86_400)
        XCTAssertFalse(TokenRefreshPolicy.needsRefresh(now: expires.addingTimeInterval(-7 * 86_400), expiresAt: expires, issuedAt: issued))
        XCTAssertTrue(TokenRefreshPolicy.needsRefresh(now: expires.addingTimeInterval(-6 * 86_400), expiresAt: expires, issuedAt: issued))
    }

    func testShortTokensRefreshAtHalfTheirLifetime() {
        let issued = Date()
        let expires = issued.addingTimeInterval(86_400)
        XCTAssertFalse(TokenRefreshPolicy.needsRefresh(now: issued.addingTimeInterval(11 * 3600), expiresAt: expires, issuedAt: issued))
        XCTAssertTrue(TokenRefreshPolicy.needsRefresh(now: issued.addingTimeInterval(12 * 3600), expiresAt: expires, issuedAt: issued))
    }

    func testNoExpiryNoRefresh() {
        XCTAssertFalse(TokenRefreshPolicy.needsRefresh(now: Date(), expiresAt: nil, issuedAt: nil))
    }
}

final class HeartbeatManagerTests: XCTestCase {
    private final class FakeAPI: DeviceAPI {
        var response = Fleetml_Device_V1_HeartbeatResponse()
        var rejectToken = false
        var unreachable = false
        var cancelled: Error?
        var delayNanoseconds: UInt64 = 0
        var heartbeats: [(token: String, request: Fleetml_Device_V1_HeartbeatRequest)] = []
        var refreshes = 0

        func heartbeat(token: String, request: Fleetml_Device_V1_HeartbeatRequest) async throws -> Fleetml_Device_V1_HeartbeatResponse {
            if unreachable { throw URLError(.cannotConnectToHost) }
            if rejectToken { throw UnauthenticatedError() }
            if let cancelled { throw cancelled }
            if delayNanoseconds > 0 { try await Task.sleep(nanoseconds: delayNanoseconds) }
            heartbeats.append((token, request))
            return response
        }

        func refreshToken(token: String) async throws -> Fleetml_Device_V1_RefreshTokenResponse {
            refreshes += 1
            var r = Fleetml_Device_V1_RefreshTokenResponse()
            r.authToken = "token-2"
            r.tokenExpiresAt = Int64(Date().addingTimeInterval(30 * 86_400).timeIntervalSince1970)
            return r
        }
    }

    private var api: FakeAPI!
    private var tokens: MemoryTokenStore!
    private var delegate: RecordingDelegate!
    private var sender: FakeSender!
    private var reporter: TelemetryReporter!
    private var models: ModelManager!

    override func setUp() {
        let dir = tempDir()
        api = FakeAPI()
        tokens = MemoryTokenStore()
        tokens.saveRegistration(deviceID: "device-1", token: "token-1",
                                expiresAtUnix: Int64(Date().addingTimeInterval(30 * 86_400).timeIntervalSince1970), signature: "")
        delegate = RecordingDelegate()
        sender = FakeSender()
        reporter = TelemetryReporter(queue: TelemetryQueue(url: dir.appendingPathComponent("q")), sender: sender,
                                     credentials: { ("device-1", "token-1") }, batchSize: 1000, flushInterval: 3600)
        let cache = ModelCache(directory: dir.appendingPathComponent("models"), maxSizeMB: 10)
        let d = delegate!
        models = ModelManager(store: ModelStore(url: dir.appendingPathComponent("models.json")), cache: cache,
                              downloader: ModelDownloader(cache: cache, configuration: StubURLProtocol.configuration()),
                              validator: NoOpModelValidator(), telemetry: reporter, delegate: { d })
    }

    private var configs: [Fleetml_Device_V1_DeviceConfig] = []

    private func manager(reRegister: @escaping () async -> Void = {}, now: @escaping () -> Date = Date.init) -> HeartbeatManager {
        let d = delegate!
        return HeartbeatManager(api: api, tokens: tokens, models: models, telemetry: reporter, delegate: { d },
                                interval: 30, reRegister: reRegister, onConfig: { [weak self] in self?.configs.append($0) }, now: now)
    }

    func testPassesTheHeartbeatsDeviceConfigOn() async {
        var config = Fleetml_Device_V1_DeviceConfig()
        config.downloadPolicy = .anyNetwork
        config.backgroundHeartbeatIntervalSeconds = 7200
        api.response.config = config
        await manager().beatOnce()
        XCTAssertEqual(configs, [config])
    }

    func testAppliesTheServerIntervalAndExperimentArms() async {
        api.response.nextHeartbeatSeconds = 120
        var arm = Fleetml_Device_V1_ExperimentAssignment()
        arm.modelVersionID = "ver-1"
        arm.armID = "arm-a"
        api.response.experiments = [arm]
        let hb = manager()

        let reached = await hb.beatOnce()
        XCTAssertTrue(reached)
        XCTAssertEqual(hb.interval, 120)
        XCTAssertEqual(api.heartbeats.first?.request.deviceID, "device-1")
        XCTAssertEqual(api.heartbeats.first?.token, "token-1")
        XCTAssertEqual(delegate.connection, [true])

        reporter.reportInference(versionID: "ver-1", latencyMs: 1)
        _ = await reporter.flushNow()
        XCTAssertEqual(sender.events.last?.experimentArmID, "arm-a")
    }

    func testRefreshesTheTokenWhenDue() async {
        let hb = manager(now: { Date().addingTimeInterval(25 * 86_400) })
        await hb.beatOnce()
        XCTAssertEqual(api.refreshes, 1)
        XCTAssertEqual(tokens.token, "token-2")
        XCTAssertEqual(api.heartbeats.first?.token, "token-2")
    }

    func testDoesNotRefreshAFreshToken() async {
        await manager().beatOnce()
        XCTAssertEqual(api.refreshes, 0)
    }

    func testARejectedTokenTriggersReRegistration() async {
        api.rejectToken = true
        var reRegistered = 0
        let hb = manager(reRegister: { reRegistered += 1 })
        let reached = await hb.beatOnce()
        XCTAssertFalse(reached)
        XCTAssertEqual(reRegistered, 1)
        XCTAssertEqual(delegate.connection, [false])
    }

    /// A heartbeat cancelled by stop() (or a cancelled gRPC call) is not a
    /// failure: the delegate does not hear connectionChanged(false).
    func testACancelledHeartbeatIsNotAFailure() async {
        let hb = manager()
        for error: Error in [CancellationError(), GRPCStatus(code: .cancelled, message: nil)] {
            api.cancelled = error
            let reached = await hb.beatOnce()
            XCTAssertFalse(reached)
        }
        XCTAssertEqual(delegate.connection, [])
        api.cancelled = GRPCStatus(code: .unavailable, message: "connection refused")
        await hb.beatOnce()
        XCTAssertEqual(delegate.connection, [false], "a real failure still counts")
    }

    /// At launch start() is called twice (start task and foreground); the
    /// second call must not cancel the first heartbeat in flight.
    func testStartingTwiceKeepsTheHeartbeatInFlight() async throws {
        api.delayNanoseconds = 200_000_000
        let hb = manager()
        hb.start()
        try await Task.sleep(nanoseconds: 50_000_000)
        hb.start()
        try await Task.sleep(nanoseconds: 400_000_000)
        hb.stop()
        XCTAssertEqual(api.heartbeats.count, 1)
        XCTAssertEqual(delegate.connection, [true])
    }

    func testReportsConnectionChangesOnce() async {
        let hb = manager()
        api.unreachable = true
        await hb.beatOnce()
        await hb.beatOnce()
        api.unreachable = false
        await hb.beatOnce()
        XCTAssertEqual(delegate.connection, [false, true])
    }

    func testReportsTheActiveModels() async {
        var a = Fleetml_Device_V1_ModelAssignment()
        let body = Data("m".utf8)
        StubURLProtocol.set("/a", .init(body: body))
        a.action = .load
        a.modelID = "m1"
        a.modelName = "iris"
        a.version = "1.0.0"
        a.variantID = "a"
        a.downloadURL = "http://models.test/a"
        a.blobSha256 = sha256Hex(body)
        api.response.assignments = [a]
        let hb = manager()
        await hb.beatOnce()
        await hb.beatOnce()
        XCTAssertEqual(api.heartbeats.last?.request.loadedModels.map(\.variantID), ["a"])
        XCTAssertEqual(api.heartbeats.last?.request.loadedModels.first?.status, "LOADED")
    }
}
