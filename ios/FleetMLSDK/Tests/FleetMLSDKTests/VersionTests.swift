import XCTest
@testable import FleetMLSDK

/// The counterpart of python/tests/test_client.py's version test: a release
/// that bumps the iOS SDK changes this line with FleetML.sdkVersion.
final class VersionTests: XCTestCase {
    func testSDKVersionIsTheRelease() {
        XCTAssertEqual(FleetML.sdkVersion, "0.10.0")
    }
}
