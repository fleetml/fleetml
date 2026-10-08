import XCTest
@testable import FleetMLSDK

final class ModelStoreTests: XCTestCase {
    private func record(_ variant: String, model: String = "m1", version: String = "1.0.0") -> ModelRecord {
        ModelRecord(modelID: model, modelName: "iris", version: version, versionID: "ver-\(variant)", variantID: variant, sha256: "x")
    }

    func testActivatingANewVersionKeepsThePreviousAsLastKnownGood() {
        let url = tempDir().appendingPathComponent("models.json")
        let store = ModelStore(url: url)
        store.activate(record("a"))
        store.activate(record("b", version: "2.0.0"))
        XCTAssertEqual(store.active("m1")?.variantID, "b")
        XCTAssertEqual(store.lastKnownGood("m1")?.variantID, "a")
        XCTAssertEqual(store.protectedVariants, ["a", "b"])

        let reopened = ModelStore(url: url)
        XCTAssertEqual(reopened.active("m1")?.variantID, "b")
        XCTAssertEqual(reopened.lastKnownGood("m1")?.variantID, "a")
    }

    func testFailedVariantsCoolDownForAnHour() {
        let store = ModelStore(url: tempDir().appendingPathComponent("models.json"))
        let t0 = Date()
        store.markFailed("bad", at: t0)
        XCTAssertTrue(store.recentlyFailed("bad", now: t0.addingTimeInterval(3599)))
        XCTAssertFalse(store.recentlyFailed("bad", now: t0.addingTimeInterval(3601)))
    }

    func testUnloadForgetsTheModel() {
        let store = ModelStore(url: tempDir().appendingPathComponent("models.json"))
        store.activate(record("a"))
        store.activate(record("b"))
        store.unload("m1")
        XCTAssertTrue(store.activeModels.isEmpty)
        XCTAssertTrue(store.protectedVariants.isEmpty)
    }
}

final class ModelDownloaderTests: XCTestCase {
    private let body = Data((0..<200_000).map { UInt8(truncatingIfNeeded: $0 &* 31) })
    private var cache: ModelCache!
    private var downloader: ModelDownloader!
    private let url = URL(string: "http://models.test/blob")!

    override func setUp() {
        StubURLProtocol.reset()
        cache = ModelCache(directory: tempDir(), maxSizeMB: 100)
        downloader = ModelDownloader(cache: cache, configuration: StubURLProtocol.configuration())
    }

    func testDownloadsAndVerifies() async throws {
        StubURLProtocol.set("/blob", .init(body: body))
        let result = await downloader.download(variant: "v1", url: url, sha256: sha256Hex(body), expectedSize: Int64(body.count))
        guard case .downloaded(let file) = result else { return XCTFail("\(result)") }
        XCTAssertEqual(try Data(contentsOf: file), body)
        let again = await downloader.download(variant: "v1", url: url, sha256: sha256Hex(body))
        XCTAssertEqual(again, .cached(file))
        XCTAssertEqual(StubURLProtocol.requests.count, 1)
    }

    func testAHashMismatchIsRejected() async {
        StubURLProtocol.set("/blob", .init(body: body))
        let result = await downloader.download(variant: "v1", url: url, sha256: String(repeating: "0", count: 64))
        XCTAssertEqual(result, .failed("SHA256 verification failed"))
        XCTAssertNil(cache.cachedFile("v1"))
        XCTAssertFalse(FileManager.default.fileExists(atPath: cache.partFile("v1").path))
    }

    func testAnInterruptedDownloadResumesWithRange() async throws {
        StubURLProtocol.set("/blob", .init(body: body, failAfter: 70_000))
        let first = await downloader.download(variant: "v1", url: url, sha256: sha256Hex(body), expectedSize: Int64(body.count))
        guard case .failed = first else { return XCTFail("expected an interruption, got \(first)") }
        let kept = try FileManager.default.attributesOfItem(atPath: cache.partFile("v1").path)[.size] as? Int
        XCTAssertEqual(kept, 70_000)

        let second = await downloader.download(variant: "v1", url: url, sha256: sha256Hex(body), expectedSize: Int64(body.count))
        guard case .downloaded(let file) = second else { return XCTFail("\(second)") }
        XCTAssertEqual(try Data(contentsOf: file), body)
        XCTAssertEqual(StubURLProtocol.requests.last?.value(forHTTPHeaderField: "Range"), "bytes=70000-")
    }

    func testAServerThatIgnoresRangeRestartsTheFile() async throws {
        StubURLProtocol.set("/blob", .init(body: body, honourRange: false, failAfter: 50_000))
        _ = await downloader.download(variant: "v1", url: url, sha256: sha256Hex(body))
        let second = await downloader.download(variant: "v1", url: url, sha256: sha256Hex(body))
        guard case .downloaded(let file) = second else { return XCTFail("\(second)") }
        XCTAssertEqual(try Data(contentsOf: file), body)
    }

    func testHTTPErrorsFail() async {
        let result = await downloader.download(variant: "v1", url: URL(string: "http://models.test/missing")!, sha256: "x")
        XCTAssertEqual(result, .failed("HTTP 404"))
    }
}

final class ModelCacheTests: XCTestCase {
    func testEvictionNeverRemovesProtectedModels() throws {
        let cache = ModelCache(directory: tempDir(), maxSizeMB: 1)
        let mb = Data(count: 600 * 1024)
        for v in ["old", "lkg", "active"] {
            let part = cache.partFile(v)
            try mb.write(to: part)
            XCTAssertNotNil(cache.commit(v, part: part, sha256: sha256Hex(mb)))
        }
        cache.evictIfNeeded(incoming: 0, protected: ["lkg", "active"])
        XCTAssertNil(cache.cachedFile("old"))
        XCTAssertNotNil(cache.cachedFile("lkg"))
        XCTAssertNotNil(cache.cachedFile("active"))
    }
}

final class ModelManagerTests: XCTestCase {
    private var dir: URL!
    private var delegate: RecordingDelegate!
    private var sender: FakeSender!
    private var reporter: TelemetryReporter!
    private var cache: ModelCache!
    private var store: ModelStore!

    override func setUp() {
        StubURLProtocol.reset()
        dir = tempDir()
        delegate = RecordingDelegate()
        sender = FakeSender()
        reporter = TelemetryReporter(queue: TelemetryQueue(url: dir.appendingPathComponent("q")), sender: sender,
                                     credentials: { ("d", "t") }, batchSize: 1000, flushInterval: 3600)
        reopen()
    }

    /// A fresh store and cache over the same directory, as after an app restart.
    private func reopen() {
        cache = ModelCache(directory: dir.appendingPathComponent("models"), maxSizeMB: 100)
        store = ModelStore(url: dir.appendingPathComponent("models.json"))
    }

    private func manager(validator: ModelValidator = NoOpModelValidator(), canDownload: @escaping (Bool) -> Bool = { _ in true }) -> ModelManager {
        let d = delegate
        return ModelManager(store: store, cache: cache, downloader: ModelDownloader(cache: cache, configuration: StubURLProtocol.configuration()),
                            validator: validator, telemetry: reporter, delegate: { d }, canDownload: canDownload)
    }

    private func assignment(_ action: Fleetml_Device_V1_ModelAssignment.Action, variant: String, version: String,
                            body: Data = Data("model".utf8)) -> Fleetml_Device_V1_ModelAssignment {
        StubURLProtocol.set("/\(variant)", .init(body: body))
        var a = Fleetml_Device_V1_ModelAssignment()
        a.action = action
        a.modelID = "m1"
        a.modelName = "iris"
        a.version = version
        a.versionID = "ver-\(variant)"
        a.variantID = variant
        a.downloadURL = "http://models.test/\(variant)"
        a.blobSha256 = sha256Hex(body)
        a.blobSizeBytes = UInt64(body.count)
        return a
    }

    private struct RejectVariant: ModelValidator {
        let variant: String
        func validate(fileURL: URL, model: ActiveModel) -> ValidationResult {
            model.variantID == variant ? .invalid("corrupt weights") : .valid
        }
    }

    func testAValidModelIsActivatedAndReported() async {
        let m = manager()
        await m.apply([assignment(.load, variant: "a", version: "1.0.0")])
        XCTAssertEqual(m.activeModels.map(\.version), ["1.0.0"])
        XCTAssertEqual(m.loadedStatuses.map(\.variantID), ["a"])
        XCTAssertEqual(delegate.ready.map(\.variantID), ["a"])
        _ = await reporter.flushNow()
        XCTAssertEqual(sender.events.map(\.type), [.modelLoad])
    }

    func testAnInvalidModelKeepsTheLastKnownGood() async {
        let m = manager(validator: RejectVariant(variant: "b"))
        await m.apply([assignment(.load, variant: "a", version: "1.0.0")])
        await m.apply([assignment(.update, variant: "b", version: "2.0.0", body: Data("broken".utf8)),
                       assignment(.unload, variant: "a", version: "1.0.0")])

        XCTAssertEqual(m.activeModels.map(\.version), ["1.0.0"])
        XCTAssertEqual(delegate.failed.map(\.reason), ["corrupt weights"])
        XCTAssertNil(cache.cachedFile("b"))
        XCTAssertTrue(store.recentlyFailed("b", now: Date()))
        _ = await reporter.flushNow()
        XCTAssertEqual(sender.events.map(\.type), [.modelLoad, .modelLoadFailed])
        XCTAssertEqual(sender.events.last?.modelVersionID, "ver-b")

        // Not retried within the cooldown.
        await m.apply([assignment(.update, variant: "b", version: "2.0.0", body: Data("broken".utf8))])
        XCTAssertEqual(delegate.failed.count, 1)
    }

    func testUpdateReplacesAndKeepsThePreviousAsLastKnownGood() async {
        let m = manager()
        await m.apply([assignment(.load, variant: "a", version: "1.0.0")])
        await m.apply([assignment(.update, variant: "b", version: "2.0.0", body: Data("v2".utf8)),
                       assignment(.unload, variant: "a", version: "1.0.0")])
        XCTAssertEqual(m.activeModels.map(\.version), ["2.0.0"])
        XCTAssertEqual(store.lastKnownGood("m1")?.variantID, "a")
        XCTAssertNotNil(cache.cachedFile("a"))
        XCTAssertTrue(delegate.unloaded.isEmpty)
    }

    func testUnloadWithoutAReplacementRemovesTheModel() async {
        let m = manager()
        await m.apply([assignment(.load, variant: "a", version: "1.0.0")])
        await m.apply([assignment(.unload, variant: "a", version: "1.0.0")])
        XCTAssertTrue(m.activeModels.isEmpty)
        XCTAssertEqual(delegate.unloaded, ["a"])
    }

    func testDownloadsWaitWhenNotAllowed() async {
        let m = manager(canDownload: { _ in false })
        await m.apply([assignment(.load, variant: "a", version: "1.0.0")])
        XCTAssertTrue(m.activeModels.isEmpty)
        XCTAssertTrue(StubURLProtocol.requests.isEmpty)
    }

    func testAnUrgentAssignmentDownloadsOnAnExpensiveNetwork() async {
        let cellular = DeviceConditions(connected: true, expensive: true, charging: false)
        let m = manager(canDownload: { urgent in BatteryPolicy.mayDownload(.unmeteredOrCharging, urgent: urgent, conditions: cellular) })
        await m.apply([assignment(.load, variant: "a", version: "1.0.0")])
        XCTAssertTrue(m.activeModels.isEmpty, "not urgent: waits")
        var urgent = assignment(.load, variant: "a", version: "1.0.0")
        urgent.urgent = true
        await m.apply([urgent])
        XCTAssertEqual(m.activeModels.map(\.version), ["1.0.0"])
    }

    func testActiveModelsSurviveARestart() async {
        await manager().apply([assignment(.load, variant: "a", version: "1.0.0")])
        reopen()
        let restarted = manager()
        XCTAssertEqual(restarted.activeModels.map(\.version), ["1.0.0"])
        XCTAssertEqual(restarted.activeModels.first.map { FileManager.default.fileExists(atPath: $0.fileURL.path) }, true)
    }
}
