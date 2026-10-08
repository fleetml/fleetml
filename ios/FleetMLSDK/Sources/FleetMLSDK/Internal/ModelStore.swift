import Foundation

/// A model version the device has (or had) running.
struct ModelRecord: Codable, Equatable {
    let modelID: String
    let modelName: String
    let version: String
    let versionID: String
    let variantID: String
    let sha256: String
}

/// Persists, per model, the active version and the last-known-good one, plus
/// variants that failed validation. Survives app termination, so the app can
/// start with its cached model offline.
final class ModelStore {
    static let failedCooldown: TimeInterval = 3600

    private struct State: Codable {
        var active: [String: ModelRecord] = [:]
        var lastKnownGood: [String: ModelRecord] = [:]
        var failed: [String: Date] = [:]
        var signingKeys: [SigningKey] = []

        init() {}

        // Files written before a field existed still load.
        init(from decoder: Decoder) throws {
            let c = try decoder.container(keyedBy: CodingKeys.self)
            active = try c.decodeIfPresent([String: ModelRecord].self, forKey: .active) ?? [:]
            lastKnownGood = try c.decodeIfPresent([String: ModelRecord].self, forKey: .lastKnownGood) ?? [:]
            failed = try c.decodeIfPresent([String: Date].self, forKey: .failed) ?? [:]
            signingKeys = try c.decodeIfPresent([SigningKey].self, forKey: .signingKeys) ?? []
        }
    }

    private let url: URL
    private let lock = NSLock()
    private var state = State()

    init(url: URL) {
        self.url = url
        try? FileManager.default.createDirectory(at: url.deletingLastPathComponent(), withIntermediateDirectories: true)
        if let data = try? Data(contentsOf: url), let s = try? JSONDecoder().decode(State.self, from: data) {
            state = s
        }
    }

    var activeModels: [ModelRecord] { lock.withLock { Array(state.active.values).sorted { $0.modelName < $1.modelName } } }
    func active(_ modelID: String) -> ModelRecord? { lock.withLock { state.active[modelID] } }
    func lastKnownGood(_ modelID: String) -> ModelRecord? { lock.withLock { state.lastKnownGood[modelID] } }
    func isActive(variantID: String) -> Bool { lock.withLock { state.active.values.contains { $0.variantID == variantID } } }

    /// Variants eviction must keep: every active and last-known-good model.
    var protectedVariants: Set<String> {
        lock.withLock { Set(state.active.values.map(\.variantID) + state.lastKnownGood.values.map(\.variantID)) }
    }

    /// Makes `record` active; the model's previous active version becomes last-known-good.
    func activate(_ record: ModelRecord) {
        lock.withLock {
            if let previous = state.active[record.modelID], previous.variantID != record.variantID {
                state.lastKnownGood[record.modelID] = previous
            }
            state.active[record.modelID] = record
            state.failed[record.variantID] = nil
            persist()
        }
    }

    func unload(_ modelID: String) {
        lock.withLock {
            state.active[modelID] = nil
            state.lastKnownGood[modelID] = nil
            persist()
        }
    }

    /// The project's signing keys, as last sent by the server.
    var signingKeys: [SigningKey] { lock.withLock { state.signingKeys } }

    /// Replaces the stored signing keys (from Register and every heartbeat).
    func setSigningKeys(_ keys: [SigningKey]) {
        lock.withLock {
            guard keys != state.signingKeys else { return }
            state.signingKeys = keys
            persist()
        }
    }

    func markFailed(_ variantID: String, at date: Date) {
        lock.withLock {
            state.failed[variantID] = date
            persist()
        }
    }

    func recentlyFailed(_ variantID: String, now: Date) -> Bool {
        lock.withLock { state.failed[variantID].map { now.timeIntervalSince($0) < Self.failedCooldown } ?? false }
    }

    private func persist() {
        if let data = try? JSONEncoder().encode(state) { try? data.write(to: url, options: .atomic) }
    }
}
