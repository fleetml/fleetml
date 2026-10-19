import Foundation
import Network
#if canImport(UIKit)
import UIKit
#endif
#if canImport(BackgroundTasks) && os(iOS)
import BackgroundTasks
#endif

/// FleetML iOS SDK entry point.
///
/// ```swift
/// try FleetML.initialize(config: FleetMLConfig(serverHost: "fleetml.example.com", enrollmentKey: key), delegate: self)
/// ```
///
/// The SDK is offline-first: models activated in an earlier run are delivered
/// through `fleetML(modelReady:)` (and `FleetML.activeModels`) as soon as it
/// starts, before the server is reached.
///
/// Battery and data: heartbeats and telemetry flushes run on a timer only while
/// the app is in the foreground. In the background a `BGAppRefreshTask`
/// (`backgroundTaskIdentifier`, at most every 15 minutes, by default hourly)
/// runs one heartbeat and one telemetry flush. Models download only as the
/// `DownloadPolicy` allows, unless the deployment is urgent.
public enum FleetML {
    public static let sdkVersion = "0.5.0"

    /// The identifier of the background refresh task. Add it to the app's
    /// `BGTaskSchedulerPermittedIdentifiers`, enable the "fetch" background mode,
    /// and call `registerBackgroundTask()` before the app finishes launching.
    public static let backgroundTaskIdentifier = "com.fleetml.sdk.refresh"

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
                                    tokens: KeychainTokenStore(), conditions: nil)
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

    /// Runs one heartbeat and one telemetry flush now, whatever the app's state
    /// (the background task calls it; apps can too, e.g. on pull to refresh).
    /// Returns true if the server was reached; false before `initialize`.
    public static func syncNow() async -> Bool {
        guard let inst = lock.withLock({ instance }) else { return false }
        return await inst.syncNow()
    }

    /// Registers the background refresh task. Call it once, before the app
    /// finishes launching (e.g. in your `App`'s `init` or
    /// `application(_:didFinishLaunchingWithOptions:)`), and initialize FleetML at
    /// launch too, so a background launch has an instance to sync.
    public static func registerBackgroundTask() {
        #if canImport(BackgroundTasks) && os(iOS)
        BGTaskScheduler.shared.register(forTaskWithIdentifier: backgroundTaskIdentifier, using: nil) { task in
            guard let task = task as? BGAppRefreshTask else { return }
            let inst = lock.withLock { instance }
            inst?.scheduleBackgroundRefresh() // the next run, whatever happens to this one
            let work = Task {
                let reached = await inst?.syncNow() ?? false
                Log.sdk.info("Background refresh: server \(reached ? "reached" : "unreachable", privacy: .public)")
                task.setTaskCompleted(success: reached)
            }
            task.expirationHandler = { work.cancel() }
        }
        #endif
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
    private let lock = NSLock()
    private var path = DeviceConditions(connected: true, expensive: true, charging: false) // until the first path update
    private var conditionsOverride: (() -> DeviceConditions)?
    private let fingerprintOverride: String?
    private var downloadPolicy: DownloadPolicy
    private var backgroundInterval: TimeInterval
    private var registered = false
    private var foreground = false
    private var observers: [NSObjectProtocol] = []
    private var startTask: Task<Void, Never>?
    /// The wait between registration attempts (shorter in tests).
    var registrationRetryNanoseconds: UInt64 = 10_000_000_000

    /// - Parameters:
    ///   - conditions: overrides the network and battery readings (tests).
    ///   - fingerprint: overrides the device fingerprint (tests).
    init(config: FleetMLConfig, delegate: FleetMLDelegate?, validator: ModelValidator, directory: URL, tokens: TokenStore,
         conditions: (() -> DeviceConditions)?, fingerprint: String? = nil) throws {
        self.config = config
        self.delegate = delegate
        self.directory = directory
        self.tokens = tokens
        conditionsOverride = conditions
        fingerprintOverride = fingerprint
        downloadPolicy = config.downloadPolicy
        backgroundInterval = config.backgroundHeartbeatIntervalSeconds
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
            canDownload: { urgent in weakSelf()?.mayDownload(urgent: urgent) ?? false })
        let api = grpc
        heartbeat = HeartbeatManager(api: api, tokens: tokens, models: models, telemetry: telemetry,
                                     delegate: { weakSelf()?.delegate }, interval: config.heartbeatIntervalSeconds,
                                     reRegister: {
                                         guard let me = weakSelf() else { return }
                                         try? await me.registration.register { try await me.register() }
                                     },
                                     onConfig: { weakSelf()?.apply($0) })
        weakSelf = { [weak self] in self }
        pathMonitor.pathUpdateHandler = { [weak self] p in
            guard let self else { return }
            self.lock.withLock {
                self.path.connected = p.status == .satisfied
                self.path.expensive = p.isExpensive || p.isConstrained
            }
        }
        pathMonitor.start(queue: DispatchQueue(label: "com.fleetml.sdk.path"))
    }

    private var conditions: DeviceConditions {
        if let conditionsOverride { return conditionsOverride() }
        return lock.withLock { path }
    }

    private func mayDownload(urgent: Bool) -> Bool {
        BatteryPolicy.mayDownload(lock.withLock { downloadPolicy }, urgent: urgent, conditions: conditions)
    }


    /// Applies the server's DeviceConfig (from registration and every heartbeat).
    private func apply(_ c: Fleetml_Device_V1_DeviceConfig) {
        if c.heartbeatIntervalSeconds > 0 { heartbeat.setInterval(TimeInterval(c.heartbeatIntervalSeconds)) }
        telemetry.configure(batchSize: Int(c.telemetryBatchSize), flushInterval: TimeInterval(c.telemetryFlushIntervalSeconds))
        telemetry.setFeatureBins(c.featureBins)
        lock.withLock {
            downloadPolicy = BatteryPolicy.fromProto(c.downloadPolicy, fallback: downloadPolicy)
            if c.backgroundHeartbeatIntervalSeconds > 0 { backgroundInterval = TimeInterval(c.backgroundHeartbeatIntervalSeconds) }
        }
    }

    private var signature: String {
        FleetML.sdkVersion + "|" + config.labels.sorted { $0.key < $1.key }.map { "\($0.key)=\($0.value)" }.joined(separator: ",")
    }

    private var needsRegistration: Bool { !tokens.isRegistered || tokens.registrationSignature != signature }
    private let registration = RegistrationGate()

    func start() {
        observeLifecycle()
        startTask?.cancel()
        startTask = Task { [weak self] in
            guard let self else { return }
            // Offline start: what was active last time is usable immediately.
            for model in self.models.activeModels { self.delegate?.fleetML(modelReady: model) }
            var lastFailure: String?
            while self.needsRegistration && !Task.isCancelled {
                do {
                    try await self.registration.register { try await self.register() }
                } catch {
                    let failure = String(describing: error)
                    Log.sdk.error("Registration failed: \(failure, privacy: .public)")
                    self.delegate?.fleetML(connectionChanged: false)
                    // The developer sees why, once per distinct error, not every retry.
                    if failure != lastFailure {
                        lastFailure = failure
                        self.delegate?.fleetML(connectionFailed: error)
                    }
                    try? await Task.sleep(nanoseconds: self.registrationRetryNanoseconds)
                }
            }
            guard !Task.isCancelled else { return }
            let runTimers = self.lock.withLock { () -> Bool in
                self.registered = true
                return self.foreground
            }
            if runTimers { self.startTimers() }
        }
    }

    /// Foreground: timers run. Background: they stop and the background refresh task takes over.
    private func observeLifecycle() {
        #if canImport(UIKit) && os(iOS)
        let center = NotificationCenter.default
        observers = [
            center.addObserver(forName: UIApplication.willEnterForegroundNotification, object: nil, queue: .main) { [weak self] _ in self?.onForeground() },
            center.addObserver(forName: UIApplication.didEnterBackgroundNotification, object: nil, queue: .main) { [weak self] _ in self?.onBackground() },
            center.addObserver(forName: UIDevice.batteryStateDidChangeNotification, object: nil, queue: .main) { [weak self] _ in self?.readBattery() },
        ]
        DispatchQueue.main.async { [weak self] in
            UIDevice.current.isBatteryMonitoringEnabled = true
            self?.readBattery()
            if UIApplication.shared.applicationState == .background { self?.onBackground() } else { self?.onForeground() }
        }
        #else
        onForeground() // no app lifecycle (macOS tools, tests): always in the foreground
        #endif
    }

    #if canImport(UIKit) && os(iOS)
    /// Called on the main thread; the download policy reads the cached value.
    private func readBattery() {
        let state = UIDevice.current.batteryState
        lock.withLock { path.charging = state == .charging || state == .full }
    }
    #endif

    private func onForeground() {
        let runTimers = lock.withLock { () -> Bool in
            foreground = true
            return registered
        }
        #if canImport(BackgroundTasks) && os(iOS)
        BGTaskScheduler.shared.cancel(taskRequestWithIdentifier: FleetML.backgroundTaskIdentifier)
        #endif
        if runTimers { startTimers() }
    }

    private func onBackground() {
        lock.withLock { foreground = false }
        heartbeat.stop()
        telemetry.stop()
        #if canImport(UIKit) && os(iOS)
        // Ask for a little time to send what is queued.
        var bgTask = UIBackgroundTaskIdentifier.invalid
        bgTask = UIApplication.shared.beginBackgroundTask { UIApplication.shared.endBackgroundTask(bgTask) }
        Task { [telemetry] in
            await telemetry.flushNow()
            UIApplication.shared.endBackgroundTask(bgTask)
        }
        #endif
        scheduleBackgroundRefresh()
    }

    private func startTimers() {
        telemetry.start()
        heartbeat.start()
    }

    /// Asks iOS for the next background refresh, no earlier than the background interval.
    func scheduleBackgroundRefresh() {
        #if canImport(BackgroundTasks) && os(iOS)
        let interval = BatteryPolicy.backgroundInterval(lock.withLock { backgroundInterval })
        let request = BGAppRefreshTaskRequest(identifier: FleetML.backgroundTaskIdentifier)
        request.earliestBeginDate = Date(timeIntervalSinceNow: interval)
        do {
            try BGTaskScheduler.shared.submit(request)
            Log.sdk.info("In the background: next refresh in \(Int(interval), privacy: .public)s or later")
        } catch {
            Log.sdk.warning("Background refresh not scheduled: \(error.localizedDescription, privacy: .public)")
        }
        #endif
    }

    /// One heartbeat and one telemetry flush (the background task and `FleetML.syncNow()`).
    func syncNow() async -> Bool {
        if needsRegistration {
            do {
                try await registration.register { try await self.register() }
                lock.withLock { registered = true }
            } catch {
                Log.sdk.warning("Registration failed: \(error.localizedDescription, privacy: .public)")
                return false
            }
        }
        let reached = await heartbeat.beatOnce()
        await telemetry.flushNow()
        return reached
    }

    func register() async throws {
        var req = Fleetml_Device_V1_RegisterRequest()
        req.fingerprint = fingerprintOverride ?? DeviceFingerprint.generate()
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
        apply(response.config) // the server's DeviceConfig wins over the local defaults
        models.setSigningKeys(response.signingKeys)
        Log.sdk.info("Registered as device \(response.deviceID, privacy: .public)")
        delegate?.fleetML(registered: response.deviceID)
        telemetry.onReconnected()
    }

    func stop() {
        observers.forEach(NotificationCenter.default.removeObserver)
        observers = []
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
