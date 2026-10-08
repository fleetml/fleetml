import XCTest
@testable import FleetMLSDK

final class TelemetryQueueTests: XCTestCase {
    func testEventsSurviveAReopen() {
        let url = tempDir().appendingPathComponent("telemetry.queue")
        let q = TelemetryQueue(url: url)
        for i in 0..<5 { q.add(event("v\(i)")) }
        q.remove(2)

        let reopened = TelemetryQueue(url: url)
        XCTAssertEqual(reopened.count, 3)
        XCTAssertEqual(reopened.peek(10).map(\.modelVersionID), ["v2", "v3", "v4"])
    }

    func testBoundDropsOldestFirst() {
        let q = TelemetryQueue(url: tempDir().appendingPathComponent("q"), maxEvents: 3)
        for i in 0..<5 { q.add(event("v\(i)")) }
        XCTAssertEqual(q.peek(10).map(\.modelVersionID), ["v2", "v3", "v4"])
        XCTAssertEqual(q.dropped, 2)
    }

    func testATornLastRecordIsIgnored() throws {
        let url = tempDir().appendingPathComponent("q")
        let q = TelemetryQueue(url: url)
        q.add(event("a"))
        q.add(event("b"))
        let data = try Data(contentsOf: url)
        try data.prefix(data.count - 2).write(to: url) // app killed mid-write
        XCTAssertEqual(TelemetryQueue(url: url).peek(10).map(\.modelVersionID), ["a"])
    }
}

final class TelemetryReporterTests: XCTestCase {
    private func reporter(_ sender: FakeSender, registered: Bool = true, batch: Int = 2) -> TelemetryReporter {
        TelemetryReporter(queue: TelemetryQueue(url: tempDir().appendingPathComponent("q")), sender: sender,
                          credentials: { registered ? ("device-1", "token") : nil },
                          batchSize: batch, flushInterval: 3600)
    }

    func testFlushSendsEverythingInBatches() async {
        let sender = FakeSender()
        let r = reporter(sender, batch: 100)
        for _ in 0..<5 { r.reportInference(versionID: "v1", latencyMs: 4, confidence: 0.9, label: "setosa") }
        r.configure(batchSize: 2, flushInterval: 0)
        let ok = await r.flushNow()
        XCTAssertTrue(ok)
        XCTAssertEqual(sender.batches.map(\.count), [2, 2, 1])
        XCTAssertEqual(r.pendingCount, 0)
        XCTAssertEqual(sender.events.first?.predictionLabel, "setosa")
        XCTAssertEqual(sender.events.first?.latencyMs, 4)
    }

    func testUnregisteredDeviceKeepsEventsQueued() async {
        let sender = FakeSender()
        let r = reporter(sender, registered: false, batch: 100)
        r.reportInference(versionID: "v1", latencyMs: 1)
        _ = await r.flushNow()
        XCTAssertTrue(sender.batches.isEmpty)
        XCTAssertEqual(r.pendingCount, 1)
    }

    func testAFailedSendKeepsTheEvents() async {
        let sender = FakeSender()
        sender.failing = true
        let r = reporter(sender, batch: 100)
        r.reportInference(versionID: "v1", latencyMs: 1)
        let ok = await r.flushNow()
        XCTAssertFalse(ok)
        XCTAssertEqual(r.pendingCount, 1)
        sender.failing = false
        let retried = await r.flushNow()
        XCTAssertTrue(retried)
        XCTAssertEqual(sender.events.count, 1)
    }

    func testEventsCarryTheExperimentArm() async {
        let sender = FakeSender()
        let r = reporter(sender, batch: 100)
        r.setExperimentArms(["v1": "arm-a"])
        r.reportInference(versionID: "v1", latencyMs: 1)
        r.reportInference(versionID: "v2", latencyMs: 1)
        _ = await r.flushNow()
        XCTAssertEqual(sender.events.map(\.experimentArmID), ["arm-a", ""])
    }

    func testModelEventTypes() async {
        let sender = FakeSender()
        let r = reporter(sender, batch: 100)
        r.reportModelEvent(versionID: "v1", .loadFailed, error: "bad file")
        r.reportModelEvent(versionID: "v1", .load, latencyMs: 12)
        _ = await r.flushNow()
        XCTAssertEqual(sender.events.map(\.type), [.modelLoadFailed, .modelLoad])
        XCTAssertEqual(sender.events.first?.errorMessage, "bad file")
    }

    func testBackoffDoublesUpToFiveMinutes() {
        XCTAssertEqual([1, 2, 3, 6, 7, 20].map(TelemetryReporter.backoff), [5, 10, 20, 160, 300, 300])
    }
}

final class FeatureHistogramTests: XCTestCase {
    func testFeatureValuesAreBinnedWithTheConfiguredEdges() {
        let reporter = TelemetryReporter(queue: TelemetryQueue(url: tempDir().appendingPathComponent("q")), sender: FakeSender(),
                                         credentials: { nil }, batchSize: 100, flushInterval: 3600)
        var bins = Fleetml_Device_V1_FeatureBins()
        bins.modelID = "m1"
        bins.feature = "petal_length"
        bins.edges = [1, 2]
        reporter.setFeatureBins([bins])
        let h = reporter.histograms(modelID: "m1", features: ["petal_length": [0.5, 1.5, 1.7, 9], "unknown": [1]])
        XCTAssertEqual(h.map(\.name), ["petal_length"])
        XCTAssertEqual(h.first?.counts, [1, 2, 1], "a value on an edge falls into the bin above it")
        XCTAssertTrue(reporter.histograms(modelID: "other", features: ["petal_length": [1]]).isEmpty)
        XCTAssertEqual(TelemetryReporter.bin(edges: [0], values: [.nan, -1, 0]), [1, 1])
    }
}

