import Foundation
import os
import XCTest
@testable import FleetMLSDK

/// An app without the background task (what "Configure and start" gives)
/// gets an info line with the fix, not an error on every launch.
final class BackgroundRefreshTests: XCTestCase {
    private let notPermitted = NSError(domain: "BGTaskSchedulerErrorDomain", code: 3)
    private let unavailable = NSError(domain: "BGTaskSchedulerErrorDomain", code: 1)
    private let tooMany = NSError(domain: "BGTaskSchedulerErrorDomain", code: 2)

    func testAnAppWithoutTheTaskGetsAnInfoLineWithTheFix() {
        let (level, message) = BackgroundRefresh.failure(notPermitted, permitted: false)
        XCTAssertEqual(level, .info)
        XCTAssertEqual(message, "Background refresh not scheduled (BGTaskSchedulerErrorDomain error 3): the app does not permit "
            + "com.fleetml.sdk.refresh; add BGTaskSchedulerPermittedIdentifiers and call registerBackgroundTask(), "
            + "see docs/IOS_SDK.md \"Battery and data\"")
    }

    func testUnavailableIsInfo() {
        XCTAssertEqual(BackgroundRefresh.failure(unavailable, permitted: true).level, .info)
    }

    func testAPermittedTaskThatStillFailsIsAnError() {
        let (level, message) = BackgroundRefresh.failure(tooMany, permitted: true)
        XCTAssertEqual(level, .error)
        XCTAssertTrue(message.hasPrefix("Background refresh not scheduled: "))
    }

    func testPermittedReadsTheInfoPlist() {
        XCTAssertFalse(BackgroundRefresh.permitted(FleetML.backgroundTaskIdentifier), "the test bundle does not list it")
    }
}
