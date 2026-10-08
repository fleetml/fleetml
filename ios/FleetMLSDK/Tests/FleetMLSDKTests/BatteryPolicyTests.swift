import XCTest
@testable import FleetMLSDK

final class BatteryPolicyTests: XCTestCase {
    private func may(_ p: DownloadPolicy, urgent: Bool = false, connected: Bool = true, expensive: Bool = false, charging: Bool = false) -> Bool {
        BatteryPolicy.mayDownload(p, urgent: urgent, conditions: DeviceConditions(connected: connected, expensive: expensive, charging: charging))
    }

    func testTheDefaultWaitsForAnInexpensiveNetworkOrACharger() {
        XCTAssertTrue(may(.unmeteredOrCharging), "Wi-Fi")
        XCTAssertFalse(may(.unmeteredOrCharging, expensive: true), "cellular on battery")
        XCTAssertTrue(may(.unmeteredOrCharging, expensive: true, charging: true), "cellular while charging")
    }

    func testUnmeteredOnlyIgnoresCharging() {
        XCTAssertFalse(may(.unmeteredOnly, expensive: true, charging: true))
        XCTAssertTrue(may(.unmeteredOnly))
    }

    func testAnyNetworkAndUrgentNeedOnlyAConnection() {
        XCTAssertTrue(may(.anyNetwork, expensive: true))
        XCTAssertTrue(may(.unmeteredOnly, urgent: true, expensive: true))
        XCTAssertFalse(may(.anyNetwork, urgent: true, connected: false), "nothing downloads offline")
    }

    func testBackgroundIntervalNeverDropsBelowTheOSMinimum() {
        XCTAssertEqual(BatteryPolicy.backgroundInterval(60), 900)
        XCTAssertEqual(BatteryPolicy.backgroundInterval(3600), 3600)
    }

    func testServerPolicyWinsUnlessUnspecified() {
        XCTAssertEqual(BatteryPolicy.fromProto(.anyNetwork, fallback: .unmeteredOnly), .anyNetwork)
        XCTAssertEqual(BatteryPolicy.fromProto(.unspecified, fallback: .unmeteredOnly), .unmeteredOnly)
    }
}
