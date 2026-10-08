import Foundation
import Network

/// FleetML iOS SDK entry point.
///
/// ```swift
/// try FleetML.initialize(config: FleetMLConfig(serverHost: "fleetml.example.com", enrollmentKey: key), delegate: self)
/// ```
///
/// The SDK is offline-first: models activated in an earlier run are delivered
/// through `fleetML(modelReady:)` (and `FleetML.activeModels`) as soon as it
/// starts, before the server is reached.
public enum FleetML {
    public static let sdkVersion = "0.1.0"

    private static let lock = NSLock()
    private static var instance: Instance?

    /// Initialize the SDK. A second call while initialized is ignored; call
    /// `stop()` first to re-initialize.
    public static func initialize(config: FleetMLConfig, delegate: FleetMLDelegate? = nil,
                                  validator: ModelValidator = ModelValidators.default(),
                                  storageDirectory: URL? = nil) throws {
        try lock.withLock {
            guard instance == nil else { return }
            let dir = storageDirectory ?? FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask)[0]
                .appendingPathComponent("fleetml", isDirectory: true)
            let inst = try Instance(config: config, delegate: delegate, validator: validator, directory: dir,
                                    tokens: KeychainTokenStore())
            instance = inst
            if config.autoStart { inst.start() }
        }
    }

    /// Start the SDK (only needed with `autoStart: false`).
    public static func start() { lock.withLock { instance }?.start() }

    /// Stop background work; `initialize` can be called again afterwards.
    public static func stop() {
        lock.withLock {
            instance?.stop()
            instance = nil
        }
    }

    /// Stop and delete cached models and the registration.
    public static func reset() {
        lock.withLock {
            instance?.reset()
            instance = nil
        }
    }

    public static var isInitialized: Bool { lock.withLock { instance != nil } }

    /// The models currently active on this device (validated, on disk).
    public static var activeModels: [ActiveModel] { lock.withLock { instance }?.models.activeModels ?? [] }

    public static func activeModel(named name: String) -> ActiveModel? { activeModels.first { $0.modelName == name } }

    /// The device ID assigned by the server, or nil before the first registration.
    public static var deviceID: String? { lock.withLock { instance }?.tokens.deviceID }

    /// The telemetry reporter (events are queued even before registration).
    public static var telemetry: TelemetryReporter? { lock.withLock { instance }?.telemetry }
}

final class Instance {
    let config: FleetMLConfig
    let tokens: TokenStore
    let telemetry: TelemetryReporter
    let models: ModelManager
    private let cache: ModelCache
    private let grpc: FleetMLRPC
    private let heartbeat: HeartbeatManager
    private weak var delegate: FleetMLDelegate?
    private let directory: URL
    private let pathMonitor = NWPathMonitor()
    private var expensiveNetwork = false
    private var startTask: Task<Void, Never>?

    init(config: FleetMLConfig, delegate: FleetMLDelegate?, validator: ModelValidator, directory: URL, tokens: TokenStore) throws {
        self.config = config
        self.delegate = delegate
        self.directory = directory
        self.tokens = tokens
        grpc = try FleetMLRPC(config: config)
        cache = ModelCache(directory: directory.appendingPathComponent("models", isDirectory: true), maxSizeMB: config.maxCacheSizeMB)
        cache.cleanupParts()
        let tokenStore = tokens
        telemetry = TelemetryReporter(
            queue: TelemetryQueue(url: directory.appendingPathComponent("telemetry.queue")),
            sender: grpc,
            credentials: { tokenStore.deviceID.flatMap { id in tokenStore.token.map { (id, $0) } } },
            batchSize: config.telemetryBatchSize, flushInterval: config.telemetryFlushIntervalSeconds)
        var weakSelf: (() -> Instance?)!
        models = ModelManager(
            store: ModelStore(url: directory.appendingPathComponent("models.json")),
            cache: cache, downloader: ModelDownloader(cache: cache), validator: validator, telemetry: telemetry,
            delegate: { weakSelf()?.delegate },
            canDownload: { !config.downloadOnWiFiOnly || !(weakSelf()?.expensiveNetwork ?? false) })
        let api = grpc
        heartbeat = HeartbeatManager(api: api, tokens: tokens, models: models, telemetry: telemetry,
                                     delegate: { weakSelf()?.delegate }, interval: config.heartbeatIntervalSeconds,
                                     reRegister: { try? await weakSelf()?.register() })
        weakSelf = { [weak self] in self }
        pathMonitor.pathUpdateHandler = { [weak self] path in self?.expensiveNetwork = path.isExpensive }
        pathMonitor.start(queue: DispatchQueue(label: "com.fleetml.sdk.path"))
    }

    private var signature: String {
        FleetML.sdkVersion + "|" + config.labels.sorted { $0.key < $1.key }.map { "\($0.key)=\($0.value)" }.joined(separator: ",")
    }

    private var needsRegistration: Bool { !tokens.isRegistered || tokens.registrationSignature != signature }

    func start() {
        telemetry.start()
        startTask?.cancel()
        startTask = Task { [weak self] in
            guard let self else { return }
            // Offline start: what was active last time is usable immediately.
            for model in self.models.activeModels { self.delegate?.fleetML(modelReady: model) }
            while self.needsRegistration && !Task.isCancelled {
                do {
                    try await self.register()
                } catch {
                    Log.sdk.error("Registration failed: \(error.localizedDescription)")
                    self.delegate?.fleetML(connectionChanged: false)
                    try? await Task.sleep(nanoseconds: 10_000_000_000)
                }
            }
            if !Task.isCancelled { self.heartbeat.start() }
        }
    }

    func register() async throws {
        var req = Fleetml_Device_V1_RegisterRequest()
        req.fingerprint = DeviceFingerprint.generate()
        req.os = "IOS"
        req.osVersion = DeviceFingerprint.osVersion
        req.chipset = DeviceFingerprint.hardwareModel()
        req.ramMb = DeviceFingerprint.ramMB
        req.hasNpu_p = true // Apple Neural Engine on every supported iPhone (A12+)
        req.hasGpu_p = true
        req.agentVersion = FleetML.sdkVersion
        req.labels = config.labels
        req.enrollmentKey = config.enrollmentKey
        let response = try await grpc.register(req, token: tokens.token)
        tokens.saveRegistration(deviceID: response.deviceID, token: response.authToken,
                                expiresAtUnix: response.tokenExpiresAt, signature: signature)
        heartbeat.setInterval(TimeInterval(response.config.heartbeatIntervalSeconds))
        telemetry.configure(batchSize: Int(response.config.telemetryBatchSize),
                            flushInterval: TimeInterval(response.config.telemetryFlushIntervalSeconds))
        Log.sdk.info("Registered as device \(response.deviceID)")
        delegate?.fleetML(registered: response.deviceID)
        telemetry.onReconnected()
    }

    func stop() {
        startTask?.cancel()
        heartbeat.stop()
        telemetry.stop()
        pathMonitor.cancel()
        grpc.shutdown()
    }

    func reset() {
        stop()
        tokens.clear()
        try? FileManager.default.removeItem(at: directory)
    }
}
