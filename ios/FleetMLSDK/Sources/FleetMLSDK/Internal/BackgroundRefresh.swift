import Foundation
import os

/// How a background refresh request that iOS refused is logged. An app that
/// followed "Configure and start" has not set up the background task, which
/// is a choice, not an error: it gets an info line with the steps to enable
/// it. Only an app that permits the task and still cannot schedule it sees
/// an error.
enum BackgroundRefresh {
    static let errorDomain = "BGTaskSchedulerErrorDomain"
    /// BGTaskScheduler.Error.Code.unavailable: the simulator, or Background App Refresh is off.
    static let unavailable = 1

    /// Whether the app's Info.plist lists `identifier` in BGTaskSchedulerPermittedIdentifiers.
    static func permitted(_ identifier: String, in bundle: Bundle = .main) -> Bool {
        (bundle.object(forInfoDictionaryKey: "BGTaskSchedulerPermittedIdentifiers") as? [String])?.contains(identifier) ?? false
    }

    /// Why a refresh request is not submitted at all, or nil to submit it. A
    /// permitted task without a launch handler (registerBackgroundTask() not
    /// called) makes iOS throw "No launch handler registered for task with
    /// identifier", an Objective-C exception that ends the app.
    static func skipReason(permitted: Bool, registered: Bool) -> String? {
        guard permitted && !registered else { return nil }
        return "Background refresh not scheduled: the app permits \(FleetML.backgroundTaskIdentifier) but has not called "
            + "FleetML.registerBackgroundTask(); call it before the app finishes launching, "
            + "see docs/IOS_SDK.md \"Battery and data\""
    }

    static func failure(_ error: Error, permitted: Bool) -> (level: OSLogType, message: String) {
        let ns = error as NSError
        if !permitted {
            return (.info, "Background refresh not scheduled (\(ns.domain) error \(ns.code)): the app does not permit "
                + "\(FleetML.backgroundTaskIdentifier); add BGTaskSchedulerPermittedIdentifiers and call registerBackgroundTask(), "
                + "see docs/IOS_SDK.md \"Battery and data\"")
        }
        if ns.domain == errorDomain && ns.code == unavailable {
            return (.info, "Background refresh not scheduled: background refresh is unavailable here "
                + "(the simulator, or Background App Refresh is off in Settings)")
        }
        return (.error, "Background refresh not scheduled: \(error.localizedDescription)")
    }
}
