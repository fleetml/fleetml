import XCTest
@testable import FleetMLSDK

/// The delta the server's Go code made: internal/delta/testdata.
final class DeltaTests: XCTestCase {
    private let dir = URL(fileURLWithPath: #filePath).deletingLastPathComponent()
        .appendingPathComponent("../../../../internal/delta/testdata").standardized

    func testAppliesTheServersDelta() throws {
        let out = try ModelDelta.patch(old: Data(contentsOf: dir.appendingPathComponent("old.bin")),
                                       patch: Data(contentsOf: dir.appendingPathComponent("patch.fmldelta")))
        XCTAssertEqual(out, try Data(contentsOf: dir.appendingPathComponent("new.bin")))
        XCTAssertThrowsError(try ModelDelta.patch(old: Data(), patch: Data("not a delta at all, sorry".utf8)))
    }

    func testAnUpdateWithADeltaIsBuiltFromIt() async throws {
        StubURLProtocol.reset()
        let old = try Data(contentsOf: dir.appendingPathComponent("old.bin"))
        let new = try Data(contentsOf: dir.appendingPathComponent("new.bin"))
        let patch = try Data(contentsOf: dir.appendingPathComponent("patch.fmldelta"))
        StubURLProtocol.set("/old", .init(body: old))
        StubURLProtocol.set("/new", .init(body: new))
        StubURLProtocol.set("/patch", .init(body: patch))
        let tmp = tempDir()
        let cache = ModelCache(directory: tmp.appendingPathComponent("models"), maxSizeMB: 100)
        let store = ModelStore(url: tmp.appendingPathComponent("models.json"))
        let manager = ModelManager(store: store, cache: cache, downloader: ModelDownloader(cache: cache, configuration: StubURLProtocol.configuration()),
                                   validator: NoOpModelValidator(), telemetry: nil, delegate: { nil }, canDownload: { _ in true })
        func assignment(_ variant: String, _ version: String, _ body: Data, _ action: Fleetml_Device_V1_ModelAssignment.Action) -> Fleetml_Device_V1_ModelAssignment {
            var a = Fleetml_Device_V1_ModelAssignment()
            a.action = action
            a.modelID = "m1"
            a.modelName = "iris"
            a.version = version
            a.versionID = "ver-\(version)"
            a.variantID = variant
            a.downloadURL = "http://models.test/\(variant)"
            a.blobSha256 = sha256Hex(body)
            a.blobSizeBytes = UInt64(body.count)
            return a
        }
        await manager.apply([assignment("old", "1.0.0", old, .load)])
        var update = assignment("new", "2.0.0", new, .update)
        update.deltaURL = "http://models.test/patch"
        update.deltaSha256 = sha256Hex(patch)
        update.deltaSizeBytes = UInt64(patch.count)
        update.deltaBaseSha256 = sha256Hex(old)
        await manager.apply([update])

        let active = try XCTUnwrap(manager.activeModels.first)
        XCTAssertEqual(active.version, "2.0.0")
        XCTAssertEqual(try Data(contentsOf: active.fileURL), new)
        XCTAssertEqual(StubURLProtocol.requests.compactMap { $0.url?.path }, ["/old", "/patch"], "1.0.0 in full, then only the delta")
    }
}
