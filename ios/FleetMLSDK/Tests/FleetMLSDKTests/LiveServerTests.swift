import XCTest
@testable import FleetMLSDK

/// Runs against a live server (scripts/ios-e2e.sh sets the environment); skipped otherwise.
/// The device is on an expensive network and on battery, so a non-urgent model waits and an
/// urgent one downloads.
final class LiveServerTests: XCTestCase {
    private func env(_ name: String) throws -> String {
        guard let v = ProcessInfo.processInfo.environment[name], !v.isEmpty else { throw XCTSkip("\(name) not set") }
        return v
    }

    func testUrgentOverridesTheDownloadPolicy() async throws {
        let host = try env("FLEETML_LIVE_HOST"), key = try env("FLEETML_LIVE_ENROLLMENT_KEY"), label = try env("FLEETML_LIVE_LABEL")
        let api = try env("FLEETML_LIVE_API"), adminKey = try env("FLEETML_LIVE_ADMIN_KEY")
        let project = try env("FLEETML_LIVE_PROJECT"), version = try env("FLEETML_LIVE_VERSION")
        let cellular = DeviceConditions(connected: true, expensive: true, charging: false)
        let inst = try Instance(config: FleetMLConfig(serverHost: host, enrollmentKey: key, labels: ["ios-live": label], autoStart: false),
                                delegate: nil, validator: ModelValidators.default(), directory: tempDir(), tokens: MemoryTokenStore(),
                                conditions: { cellular },
                                // A new device each run: the test runner's real fingerprint repeats across runs.
                                fingerprint: (0..<32).map { _ in String(format: "%02x", UInt8.random(in: 0...255)) }.joined())
        defer { inst.stop() }

        let reached = await inst.syncNow()
        XCTAssertTrue(reached, "registered and heartbeated")
        _ = await inst.syncNow()
        XCTAssertTrue(inst.models.activeModels.isEmpty, "a non-urgent model waits on an expensive network")

        var req = URLRequest(url: URL(string: "\(api)/api/v1/deployments")!)
        req.httpMethod = "POST"
        req.setValue("Bearer \(adminKey)", forHTTPHeaderField: "Authorization")
        req.setValue("application/json", forHTTPHeaderField: "Content-Type")
        req.httpBody = try JSONSerialization.data(withJSONObject: [
            "project_id": project, "model_version_id": version, "urgent": true, "target_selector": ["ios-live": label],
        ])
        let (data, _) = try await URLSession.shared.data(for: req)
        let id = try XCTUnwrap(((try JSONSerialization.jsonObject(with: data) as? [String: Any])?["data"] as? [String: Any])?["id"] as? String)
        var start = URLRequest(url: URL(string: "\(api)/api/v1/deployments/\(id)/start")!)
        start.httpMethod = "POST"
        start.setValue("Bearer \(adminKey)", forHTTPHeaderField: "Authorization")
        _ = try await URLSession.shared.data(for: start)

        _ = await inst.syncNow()
        XCTAssertEqual(inst.models.activeModels.first?.versionID, version, "an urgent model downloads on an expensive network")
    }

    /// One phase of scripts/signing-e2e.sh: the models in FLEETML_LIVE_EXPECT_ACTIVE must become
    /// active and those in FLEETML_LIVE_EXPECT_REJECTED must not (their signature does not verify).
    func testSignaturesAreVerifiedBeforeActivation() async throws {
        let host = try env("FLEETML_LIVE_HOST"), key = try env("FLEETML_LIVE_ENROLLMENT_KEY"), label = try env("FLEETML_LIVE_SIG_LABEL")
        let expectActive = Set(try env("FLEETML_LIVE_EXPECT_ACTIVE").split(separator: ",").map(String.init))
        let expectRejected = Set(try env("FLEETML_LIVE_EXPECT_REJECTED").split(separator: ",").map(String.init))
        let delegate = RecordingDelegate()
        let inst = try Instance(config: FleetMLConfig(serverHost: host, enrollmentKey: key, labels: ["e2e-run": label], autoStart: false),
                                delegate: delegate, validator: ModelValidators.default(), directory: tempDir(), tokens: MemoryTokenStore(),
                                conditions: { DeviceConditions(connected: true, expensive: false, charging: true) },
                                fingerprint: (0..<32).map { _ in String(format: "%02x", UInt8.random(in: 0...255)) }.joined())
        defer { inst.stop() }
        func active() -> Set<String> { Set(inst.models.activeModels.map(\.modelName)) }
        for _ in 0..<10 where !active().isSuperset(of: expectActive) {
            _ = await inst.syncNow()
        }
        XCTAssertTrue(active().isSuperset(of: expectActive), "active \(active()) contains \(expectActive)")
        XCTAssertTrue(active().isDisjoint(with: expectRejected), "none of \(expectRejected) is active: \(active())")
        for name in expectRejected {
            XCTAssertTrue(delegate.failed.contains { $0.name == name && $0.reason.hasPrefix("signature:") }, "\(name) rejected: \(delegate.failed)")
        }
        let flushed = await inst.telemetry.flushNow()
        XCTAssertTrue(flushed)
        print("FleetMLSignatureTest active=\(active().sorted()) rejected=\(delegate.failed.map(\.name))")
    }
}
