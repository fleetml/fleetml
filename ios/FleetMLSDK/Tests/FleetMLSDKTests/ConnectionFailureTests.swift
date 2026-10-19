import XCTest
@testable import FleetMLSDK

final class ConnectionFailureTests: XCTestCase {
    /// An own app pointed at a wrong host learns why through its delegate, not
    /// only through `connectionChanged(false)`: once for the first failure, not
    /// again for every retry with the same error.
    func testAWrongHostReachesTheDelegate() async throws {
        let delegate = RecordingDelegate()
        let wifi = DeviceConditions(connected: true, expensive: false, charging: true)
        // Nothing listens on port 1 of the loopback address.
        let inst = try Instance(config: FleetMLConfig(serverHost: "127.0.0.1", serverPort: 1, enrollmentKey: "k", autoStart: false),
                                delegate: delegate, validator: ModelValidators.default(), directory: tempDir(),
                                tokens: MemoryTokenStore(), conditions: { wifi }, fingerprint: String(repeating: "ab", count: 32))
        inst.registrationRetryNanoseconds = 100_000_000
        defer { inst.stop() }
        inst.start()

        let deadline = Date().addingTimeInterval(60)
        while delegate.connection.count < 2 && Date() < deadline {
            try await Task.sleep(nanoseconds: 100_000_000)
        }
        XCTAssertGreaterThanOrEqual(delegate.connection.count, 2, "registration was tried at least twice")
        XCTAssertEqual(delegate.connection, Array(repeating: false, count: delegate.connection.count))
        XCTAssertEqual(delegate.connectionFailures.count, 1, "one call per distinct error: \(delegate.connectionFailures)")
        XCTAssertFalse(delegate.connectionFailures.first?.isEmpty ?? true, "the error says what failed")
        print("connectionFailed: \(delegate.connectionFailures.first ?? "")")
    }

    /// A delegate written before connectionFailed existed still compiles: the
    /// method has a default implementation.
    func testOlderDelegatesNeedNoChange() {
        final class OldDelegate: FleetMLDelegate {
            func fleetML(connectionChanged connected: Bool) {}
        }
        OldDelegate().fleetML(connectionFailed: URLError(.cannotConnectToHost))
    }
}
