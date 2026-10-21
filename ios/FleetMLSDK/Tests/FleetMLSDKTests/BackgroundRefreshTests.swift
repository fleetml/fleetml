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

    /// Submitting a permitted task that has no launch handler makes iOS throw
    /// "No launch handler registered for task with identifier", an
    /// Objective-C exception that ends the app, so the SDK does not submit it.
    func testAPermittedTaskWithoutRegisterBackgroundTaskIsNotSubmitted() {
        XCTAssertEqual(BackgroundRefresh.skipReason(permitted: true, registered: false),
                       "Background refresh not scheduled: the app permits com.fleetml.sdk.refresh but has not called "
                       + "FleetML.registerBackgroundTask(); call it before the app finishes launching, "
                       + "see docs/IOS_SDK.md \"Battery and data\"")
        XCTAssertNil(BackgroundRefresh.skipReason(permitted: true, registered: true))
        XCTAssertNil(BackgroundRefresh.skipReason(permitted: false, registered: false),
                     "iOS refuses an unpermitted task with error 3, which is logged with the hint above")
    }

    func testPermittedReadsTheInfoPlist() {
        XCTAssertFalse(BackgroundRefresh.permitted(FleetML.backgroundTaskIdentifier), "the test bundle does not list it")
    }
}
