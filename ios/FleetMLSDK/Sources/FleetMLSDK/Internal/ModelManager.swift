import Foundation

/// Applies the server's assignments: download (resumable, sha256-verified),
/// validate, and only then activate. A model that fails validation never
/// replaces the active one; the last-known-good file is never deleted.
final class ModelManager {
    private let store: ModelStore
    private let cache: ModelCache
    private let downloader: ModelDownloader
    private let validator: ModelValidator
    private let telemetry: TelemetryReporter?
    private let delegate: () -> FleetMLDelegate?
    private let canDownload: (_ urgent: Bool) -> Bool
    private let now: () -> Date

    init(store: ModelStore, cache: ModelCache, downloader: ModelDownloader, validator: ModelValidator,
         telemetry: TelemetryReporter?, delegate: @escaping () -> FleetMLDelegate?,
         canDownload: @escaping (_ urgent: Bool) -> Bool = { _ in true }, now: @escaping () -> Date = Date.init) {
        self.store = store
        self.cache = cache
        self.downloader = downloader
        self.validator = validator
        self.telemetry = telemetry
        self.delegate = delegate
        self.canDownload = canDownload
        self.now = now
    }

    /// What the device reports in its heartbeat: active (validated) models only.
    var loadedStatuses: [Fleetml_Device_V1_ModelStatus] {
        store.activeModels.map {
            var s = Fleetml_Device_V1_ModelStatus()
            s.modelID = $0.modelID
            s.version = $0.version
            s.variantID = $0.variantID
            s.status = "LOADED"
            return s
        }
    }

    /// Active models whose files are on disk (offline start).
    var activeModels: [ActiveModel] {
        store.activeModels.compactMap { rec in cache.cachedFile(rec.variantID).map { rec.active(at: $0) } }
    }

    func apply(_ assignments: [Fleetml_Device_V1_ModelAssignment]) async {
        // A replacement arrives as UPDATE(new) + UNLOAD(old); the old version
        // must stay until the new one is validated, so such UNLOADs are ignored.
        let replacing = Set(assignments.filter { $0.action == .load || $0.action == .update }.map(\.modelID))
        for a in assignments {
            switch a.action {
            case .load, .update: await install(a)
            case .unload: unload(a, replacing: replacing)
            default: break
            }
        }
    }

    private func install(_ a: Fleetml_Device_V1_ModelAssignment) async {
        if store.isActive(variantID: a.variantID) || store.recentlyFailed(a.variantID, now: now()) { return }
        if cache.cachedFile(a.variantID) == nil && !canDownload(a.urgent) {
            Log.sdk.info("Download of \(a.modelName) \(a.version) waits for the download policy (inexpensive network or charging)")
            return // the assignment is repeated on the next heartbeat
        }
        guard let url = URL(string: a.downloadURL) else { return }

        let file: URL
        switch await downloader.download(variant: a.variantID, url: url, sha256: a.blobSha256,
                                         expectedSize: Int64(a.blobSizeBytes), protected: store.protectedVariants) {
        case .downloaded(let f), .cached(let f): file = f
        case .failed(let reason):
            delegate()?.fleetML(modelLoadFailed: a.modelName, version: a.version, reason: reason)
            return
        }

        let record = ModelRecord(modelID: a.modelID, modelName: a.modelName, version: a.version,
                                 versionID: a.versionID, variantID: a.variantID, sha256: a.blobSha256)
        let candidate = record.active(at: file)
        let started = now()
        switch validator.validate(fileURL: file, model: candidate) {
        case .invalid(let reason):
            Log.sdk.warning("Model \(a.modelName) \(a.version) failed validation: \(reason); keeping the current model")
            store.markFailed(a.variantID, at: now())
            if !store.protectedVariants.contains(a.variantID) { cache.delete(a.variantID) }
            telemetry?.reportModelEvent(versionID: a.versionID, .loadFailed, error: reason)
            delegate()?.fleetML(modelLoadFailed: a.modelName, version: a.version, reason: reason)
        case .valid:
            store.activate(record)
            telemetry?.reportModelEvent(versionID: a.versionID, .load, latencyMs: Int(now().timeIntervalSince(started) * 1000))
            Log.sdk.info("Model \(a.modelName) \(a.version) is active")
            delegate()?.fleetML(modelReady: candidate)
        }
    }

    private func unload(_ a: Fleetml_Device_V1_ModelAssignment, replacing: Set<String>) {
        guard let active = store.activeModels.first(where: { $0.variantID == a.variantID }),
              !replacing.contains(active.modelID) else { return }
        store.unload(active.modelID)
        delegate()?.fleetML(modelUnloaded: active.modelID, modelName: active.modelName, variantID: active.variantID)
    }
}

extension ModelRecord {
    func active(at url: URL) -> ActiveModel {
        ActiveModel(modelID: modelID, modelName: modelName, version: version, versionID: versionID,
                    variantID: variantID, sha256: sha256, fileURL: url)
    }
}
