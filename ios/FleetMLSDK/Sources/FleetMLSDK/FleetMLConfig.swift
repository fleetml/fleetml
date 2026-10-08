import Foundation

/// Configuration for the FleetML iOS SDK.
public struct FleetMLConfig {
    /// The FleetML server host (gRPC).
    public var serverHost: String
    /// The gRPC port.
    public var serverPort: Int
    /// Per-project enrollment key (`POST /api/v1/projects/{id}/enrollment-keys`).
    /// Required for the first registration; afterwards the device uses its token.
    public var enrollmentKey: String
    /// Use TLS for gRPC (put the server behind a TLS proxy in production).
    public var useTLS: Bool
    /// Labels that deployments and experiments target.
    public var labels: [String: String]
    /// Heartbeat interval while the app is in the foreground, until the server sends its own.
    public var heartbeatIntervalSeconds: TimeInterval
    /// Interval of the background refresh (one heartbeat and one telemetry flush) while the
    /// app is in the background, until the server sends its own; iOS runs it at most every
    /// 15 minutes and decides the exact time.
    public var backgroundHeartbeatIntervalSeconds: TimeInterval
    /// Disk budget for models; the active and last-known-good models are never evicted.
    public var maxCacheSizeMB: Int
    /// Which networks non-urgent model downloads may use, until the server's DeviceConfig sets it.
    public var downloadPolicy: DownloadPolicy
    /// Start registration and heartbeats as soon as the SDK is initialized.
    public var autoStart: Bool
    /// Telemetry batch size and flush interval until the server's DeviceConfig overrides them.
    public var telemetryBatchSize: Int
    public var telemetryFlushIntervalSeconds: TimeInterval

    public init(
        serverHost: String,
        serverPort: Int = 19090,
        enrollmentKey: String = "",
        useTLS: Bool = false,
        labels: [String: String] = [:],
        heartbeatIntervalSeconds: TimeInterval = 30,
        backgroundHeartbeatIntervalSeconds: TimeInterval = 3600,
        maxCacheSizeMB: Int = 500,
        downloadPolicy: DownloadPolicy = .unmeteredOrCharging,
        autoStart: Bool = true,
        telemetryBatchSize: Int = 50,
        telemetryFlushIntervalSeconds: TimeInterval = 30
    ) {
        self.serverHost = serverHost
        self.serverPort = serverPort
        self.enrollmentKey = enrollmentKey
        self.useTLS = useTLS
        self.labels = labels
        self.heartbeatIntervalSeconds = heartbeatIntervalSeconds
        self.backgroundHeartbeatIntervalSeconds = backgroundHeartbeatIntervalSeconds
        self.maxCacheSizeMB = maxCacheSizeMB
        self.downloadPolicy = downloadPolicy
        self.autoStart = autoStart
        self.telemetryBatchSize = telemetryBatchSize
        self.telemetryFlushIntervalSeconds = telemetryFlushIntervalSeconds
    }
}

/// A model the SDK has downloaded, verified (sha256) and validated, and that the
/// app should run. Pass it to `TelemetryReporter.reportInference` so events are
/// attributed to the right version (and experiment arm).
public struct ActiveModel: Equatable {
    /// The model, stable across versions.
    public let modelID: String
    public let modelName: String
    /// Human-readable version, e.g. "1.2.0".
    public let version: String
    /// The model version ID that telemetry is attributed to.
    public let versionID: String
    public let variantID: String
    public let sha256: String
    /// The model file on disk.
    public let fileURL: URL
}

/// Lifecycle events from the SDK. Every method has an empty default
/// implementation. Calls arrive on a background queue.
public protocol FleetMLDelegate: AnyObject {
    /// A model was downloaded, verified and validated and is now the active
    /// version of its model. Also called at start for models cached from an
    /// earlier run (offline start).
    func fleetML(modelReady model: ActiveModel)
    /// The server no longer assigns this model.
    func fleetML(modelUnloaded modelID: String, modelName: String, variantID: String)
    /// A new version could not be downloaded or failed validation; the
    /// previous (last-known-good) version stays active.
    func fleetML(modelLoadFailed modelName: String, version: String, reason: String)
    /// The device registered (or re-registered).
    func fleetML(registered deviceID: String)
    /// The server became reachable or unreachable.
    func fleetML(connectionChanged connected: Bool)
}

public extension FleetMLDelegate {
    func fleetML(modelReady model: ActiveModel) {}
    func fleetML(modelUnloaded modelID: String, modelName: String, variantID: String) {}
    func fleetML(modelLoadFailed modelName: String, version: String, reason: String) {}
    func fleetML(registered deviceID: String) {}
    func fleetML(connectionChanged connected: Bool) {}
}
