import XCTest
@testable import FleetMLSDK

/// A build with another key, host or port registers again; the same build does not.
final class RegistrationSignatureTests: XCTestCase {
    private let labels = ["app": "sample", "fleet": "a"]
    private func sig(host: String = "localhost", port: Int = 19090, key: String = "k1") -> String {
        Instance.registrationSignature(sdkVersion: "0.10.2", labels: labels, host: host, port: port, enrollmentKey: key)
    }

    func testTheSameSettingsGiveTheSameSignature() {
        XCTAssertEqual(sig(), sig())
        XCTAssertEqual(sig(), sig(key: " k1 "), "a key with spaces around is the same key")
    }

    func testANewKeyHostOrPortChangesIt() {
        XCTAssertNotEqual(sig(), sig(key: "k2"))
        XCTAssertNotEqual(sig(), sig(host: "192.0.2.10"))
        XCTAssertNotEqual(sig(), sig(port: 29090))
    }

    func testTheKeyItselfIsNotInIt() {
        XCTAssertFalse(sig(key: "secret-enrollment-key").contains("secret-enrollment-key"))
    }

    func testASignatureOfAnOlderSDKDiffers() {
        XCTAssertNotEqual("0.10.2|app=sample,fleet=a", sig())
    }
}
