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

    /// Stores the project's signing keys from a Register or heartbeat response.
    func setSigningKeys(_ keys: [Fleetml_Device_V1_SigningKey]) {
        store.setSigningKeys(keys.map { SigningKey(keyID: $0.keyID, publicKey: $0.publicKey, notAfter: $0.notAfter) })
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
        // The signature covers the sha256 the download is checked against, so
        // a model that fails it is not even downloaded.
        if let reason = ArtifactSignature.verify(keys: store.signingKeys, keyID: a.signingKeyID, signature: a.signature,
                                                 modelID: a.modelID, version: a.version, sha256: a.blobSha256, now: now()) {
            Log.sdk.warning("Model \(a.modelName, privacy: .public) \(a.version, privacy: .public) rejected: \(reason, privacy: .public); keeping the current model")
            store.markFailed(a.variantID, at: now())
            telemetry?.reportModelEvent(versionID: a.versionID, .signatureInvalid, error: reason)
            delegate()?.fleetML(modelLoadFailed: a.modelName, version: a.version, reason: "signature: \(reason)")
            return
        }
        if cache.cachedFile(a.variantID) == nil && !canDownload(a.urgent) {
            Log.sdk.info("Download of \(a.modelName, privacy: .public) \(a.version, privacy: .public) waits for the download policy (inexpensive network or charging)")
            return // the assignment is repeated on the next heartbeat
        }
        guard let url = URL(string: a.downloadURL) else { return }

        let file: URL
        if let built = await fromDelta(a) {
            file = built
        } else {
            Log.sdk.info("Downloading \(a.modelName, privacy: .public) \(a.version, privacy: .public) (\(a.blobSizeBytes, privacy: .public) bytes)")
            switch await downloader.download(variant: a.variantID, url: url, sha256: a.blobSha256,
                                             expectedSize: Int64(a.blobSizeBytes), protected: store.protectedVariants) {
            case .downloaded(let f):
                Log.sdk.info("Downloaded \(a.modelName, privacy: .public) \(a.version, privacy: .public) (\(a.blobSizeBytes, privacy: .public) bytes, sha256 verified)")
                file = f
            case .cached(let f):
                Log.sdk.info("\(a.modelName, privacy: .public) \(a.version, privacy: .public) is already downloaded")
                file = f
            case .failed(let reason):
                Log.sdk.warning("Download of \(a.modelName, privacy: .public) \(a.version, privacy: .public) failed: \(reason, privacy: .public); retried on the next heartbeat")
                delegate()?.fleetML(modelLoadFailed: a.modelName, version: a.version, reason: reason)
                return
            }
        }

        let record = ModelRecord(modelID: a.modelID, modelName: a.modelName, version: a.version,
                                 versionID: a.versionID, variantID: a.variantID, sha256: a.blobSha256)
        let candidate = record.active(at: file)
        let started = now()
        switch validator.validate(fileURL: file, model: candidate) {
        case .invalid(let reason):
            Log.sdk.warning("Model \(a.modelName, privacy: .public) \(a.version, privacy: .public) failed validation: \(reason, privacy: .public); keeping the current model")
            store.markFailed(a.variantID, at: now())
            if !store.protectedVariants.contains(a.variantID) { cache.delete(a.variantID) }
            telemetry?.reportModelEvent(versionID: a.versionID, .loadFailed, error: reason)
            delegate()?.fleetML(modelLoadFailed: a.modelName, version: a.version, reason: reason)
        case .valid:
            Log.sdk.info("Model \(a.modelName, privacy: .public) \(a.version, privacy: .public) passed validation")
            store.activate(record)
            telemetry?.reportModelEvent(versionID: a.versionID, .load, latencyMs: Int(now().timeIntervalSince(started) * 1000))
            Log.sdk.info("Model \(a.modelName, privacy: .public) \(a.version, privacy: .public) is active")
            delegate()?.fleetML(modelReady: candidate)
        }
    }

    /// Builds the model from the assignment's delta and the version this device has (active or
    /// last-known-good). Returns nil when there is no usable delta or anything fails; the full
    /// model is downloaded then.
    private func fromDelta(_ a: Fleetml_Device_V1_ModelAssignment) async -> URL? {
        guard !a.deltaURL.isEmpty, cache.cachedFile(a.variantID) == nil, let deltaURL = URL(string: a.deltaURL),
              let base = [store.active(a.modelID), store.lastKnownGood(a.modelID)].compactMap({ $0 })
                .first(where: { $0.sha256.caseInsensitiveCompare(a.deltaBaseSha256) == .orderedSame }),
              let baseFile = cache.cachedFile(base.variantID) else { return nil }
        let deltaID = "delta-\(a.variantID)"
        defer { cache.delete(deltaID) }
        guard case .downloaded(let deltaFile) = await downloader.download(variant: deltaID, url: deltaURL, sha256: a.deltaSha256,
                                                                          expectedSize: Int64(a.deltaSizeBytes), protected: store.protectedVariants) else {
            Log.sdk.warning("Delta for \(a.modelName, privacy: .public) \(a.version, privacy: .public) failed; downloading the full model")
            return nil
        }
        do {
            let patch = try Data(contentsOf: deltaFile)
            let out = try ModelDelta.patch(old: try Data(contentsOf: baseFile), patch: patch)
            let part = cache.partFile(a.variantID)
            try out.write(to: part)
            guard let file = cache.commit(a.variantID, part: part, sha256: a.blobSha256) else {
                Log.sdk.warning("Model \(a.modelName, privacy: .public) \(a.version, privacy: .public) built from a delta does not match its sha256; downloading the full model")
                return nil
            }
            Log.sdk.info("Model \(a.modelName, privacy: .public) \(a.version, privacy: .public) built from a \(patch.count, privacy: .public)-byte delta instead of \(a.blobSizeBytes, privacy: .public) bytes")
            return file
        } catch {
            Log.sdk.warning("Delta for \(a.modelName, privacy: .public) \(a.version, privacy: .public) unusable (\(String(describing: error), privacy: .public)); downloading the full model")
            return nil
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
