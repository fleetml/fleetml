import Foundation

/// Sends a batch of telemetry for a registered device; returns the accepted count.
protocol TelemetrySender {
    func send(deviceID: String, token: String, events: [Fleetml_Telemetry_V1_TelemetryEvent]) async throws -> Int
}

/// Reports inference telemetry to the FleetML server.
///
/// Events go into a persistent, bounded queue first: they survive app
/// termination and are kept while offline or not yet registered. A background
/// loop sends them in batches every flush interval, backs off exponentially
/// (5 s doubling to 5 min) after a failed send, and flushes as soon as the
/// connection returns. Events for a version in an A/B experiment carry the arm ID.
public final class TelemetryReporter {
    /// Event types a lifecycle report can carry.
    public enum ModelEvent { case load, loadFailed, oom, error, signatureInvalid }

    private let queue: TelemetryQueue
    private let sender: TelemetrySender
    private let credentials: () -> (deviceID: String, token: String)?
    private let now: () -> Date
    private let lock = NSLock()
    private var batchSize: Int
    private var flushInterval: TimeInterval
    private var arms: [String: String] = [:]
    private var loop: Task<Void, Never>?
    private var flushing = false

    init(queue: TelemetryQueue, sender: TelemetrySender, credentials: @escaping () -> (deviceID: String, token: String)?,
         batchSize: Int, flushInterval: TimeInterval, now: @escaping () -> Date = Date.init) {
        self.queue = queue
        self.sender = sender
        self.credentials = credentials
        self.batchSize = max(1, batchSize)
        self.flushInterval = max(1, flushInterval)
        self.now = now
    }

    /// Events waiting to be sent.
    public var pendingCount: Int { queue.count }

    /// Report one inference by an active model.
    ///
    /// `features` are the model's input values by feature name: each feature configured on the
    /// server (`PUT /models/{id}/features`) is binned into a histogram for feature-level drift;
    /// other names are ignored.
    public func reportInference(model: ActiveModel, latencyMs: Int, confidence: Float = 0, label: String = "", error: String = "",
                                features: [String: [Float]] = [:]) {
        var e = event(versionID: model.versionID, type: .inference, latencyMs: latencyMs, error: error)
        e.confidenceScore = confidence
        e.predictionLabel = label
        e.features = histograms(modelID: model.modelID, features: features)
        enqueue(e)
    }

    /// Report one inference for a model version ID.
    public func reportInference(versionID: String, latencyMs: Int, confidence: Float = 0, label: String = "", error: String = "") {
        var e = event(versionID: versionID, type: .inference, latencyMs: latencyMs, error: error)
        e.confidenceScore = confidence
        e.predictionLabel = label
        enqueue(e)
    }

    /// Bin edges per model ID and feature name, from the server's DeviceConfig.
    private var featureBins: [String: [String: [Double]]] = [:]

    func setFeatureBins(_ bins: [Fleetml_Device_V1_FeatureBins]) {
        let grouped = Dictionary(grouping: bins, by: \.modelID).mapValues { Dictionary($0.map { ($0.feature, $0.edges) }, uniquingKeysWith: { a, _ in a }) }
        lock.withLock { featureBins = grouped }
    }

    func histograms(modelID: String, features: [String: [Float]]) -> [Fleetml_Telemetry_V1_FeatureHistogram] {
        guard let bins = lock.withLock({ featureBins[modelID] }) else { return [] }
        return features.sorted { $0.key < $1.key }.compactMap { name, values in
            guard let edges = bins[name] else { return nil }
            var h = Fleetml_Telemetry_V1_FeatureHistogram()
            h.name = name
            h.counts = Self.bin(edges: edges, values: values)
            return h
        }
    }

    /// Counts values into edges.count + 1 bins (below the first edge, between edges, at or above the last); NaN is skipped.
    static func bin(edges: [Double], values: [Float]) -> [UInt32] {
        var counts = [UInt32](repeating: 0, count: edges.count + 1)
        for v in values where !v.isNaN {
            counts[edges.firstIndex { $0 > Double(v) } ?? edges.count] += 1
        }
        return counts
    }

    /// Report a model lifecycle event.
    public func reportModelEvent(versionID: String, _ type: ModelEvent, latencyMs: Int = 0, error: String = "") {
        let t: Fleetml_Telemetry_V1_TelemetryEvent.EventType
        switch type {
        case .load: t = .modelLoad
        case .loadFailed: t = .modelLoadFailed
        case .oom: t = .oom
        case .error: t = .error
        case .signatureInvalid: t = .signatureInvalid
        }
        enqueue(event(versionID: versionID, type: t, latencyMs: latencyMs, error: error))
    }

    private func event(versionID: String, type: Fleetml_Telemetry_V1_TelemetryEvent.EventType, latencyMs: Int, error: String) -> Fleetml_Telemetry_V1_TelemetryEvent {
        var e = Fleetml_Telemetry_V1_TelemetryEvent()
        e.eventID = UUID().uuidString // stored once, even if the batch is sent again
        e.modelVersionID = versionID
        e.experimentArmID = lock.withLock { arms[versionID] ?? "" }
        e.type = type
        e.latencyMs = UInt32(max(0, latencyMs))
        e.errorMessage = error
        e.timestampUnixMs = Int64(now().timeIntervalSince1970 * 1000)
        return e
    }

    private func enqueue(_ e: Fleetml_Telemetry_V1_TelemetryEvent) {
        queue.add(e)
        if queue.count >= lock.withLock({ batchSize }) { Task { await self.flushNow() } }
    }

    func setExperimentArms(_ versionToArm: [String: String]) { lock.withLock { arms = versionToArm } }

    func configure(batchSize: Int, flushInterval: TimeInterval) {
        lock.withLock {
            if batchSize > 0 { self.batchSize = batchSize }
            if flushInterval > 0 { self.flushInterval = flushInterval }
        }
    }

    func start() {
        loop?.cancel()
        loop = Task { [weak self] in
            var failures = 0
            while !Task.isCancelled {
                guard let self else { return }
                let wait = failures == 0 ? self.lock.withLock { self.flushInterval } : Self.backoff(failures)
                try? await Task.sleep(nanoseconds: UInt64(wait * 1_000_000_000))
                failures = await self.flushNow() ? 0 : failures + 1
            }
        }
    }

    func onReconnected() { Task { await self.flushNow() } }

    func stop() {
        loop?.cancel()
        loop = nil
    }

    /// Sends queued events in batches until the queue is empty; false if a send
    /// failed (events stay queued). Unregistered devices keep everything queued.
    /// One flush runs at a time: a caller that finds one running waits for it,
    /// then sends what is left, so events reported before the call are sent
    /// when it returns.
    @discardableResult
    func flushNow() async -> Bool {
        while !lock.withLock({ () -> Bool in
            if flushing { return false }
            flushing = true
            return true
        }) {
            try? await Task.sleep(nanoseconds: 20_000_000)
        }
        defer { lock.withLock { flushing = false } }
        guard let creds = credentials() else { return true }
        while true {
            let batch = queue.peek(lock.withLock { batchSize })
            if batch.isEmpty { return true }
            do {
                _ = try await sender.send(deviceID: creds.deviceID, token: creds.token, events: batch)
                queue.remove(batch.count)
            } catch {
                return false
            }
        }
    }

    static func backoff(_ failures: Int) -> TimeInterval { min(300, 5 * pow(2, Double(max(0, failures - 1)))) }
}
