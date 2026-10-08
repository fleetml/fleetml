import Foundation

/// When the SDK may download a model that is not marked urgent. The server's
/// DeviceConfig overrides the value in `FleetMLConfig`; urgent assignments
/// download on any connected network.
public enum DownloadPolicy: Equatable {
    /// An inexpensive network (Wi-Fi, Ethernet), or any network while charging.
    case unmeteredOrCharging
    /// Only an inexpensive network.
    case unmeteredOnly
    /// Any connected network.
    case anyNetwork
}

/// What the download policy looks at.
struct DeviceConditions: Equatable {
    var connected: Bool
    /// Cellular, a personal hotspot, or Low Data Mode (NWPath isExpensive or isConstrained).
    var expensive: Bool
    var charging: Bool
}

/// The battery and data rules, as pure functions so they are easy to test.
enum BatteryPolicy {
    /// iOS schedules background refresh at most every 15 minutes, usually less often.
    static let minBackgroundInterval: TimeInterval = 15 * 60

    /// Whether a model download may start now.
    static func mayDownload(_ policy: DownloadPolicy, urgent: Bool, conditions c: DeviceConditions) -> Bool {
        guard c.connected else { return false }
        if urgent { return true }
        switch policy {
        case .anyNetwork: return true
        case .unmeteredOnly: return !c.expensive
        case .unmeteredOrCharging: return !c.expensive || c.charging
        }
    }

    /// The background check interval: the server's, but never below the OS minimum.
    static func backgroundInterval(_ requested: TimeInterval) -> TimeInterval { max(minBackgroundInterval, requested) }

    /// The server's policy, or `fallback` when the server leaves it unspecified.
    static func fromProto(_ p: Fleetml_Device_V1_DownloadPolicy, fallback: DownloadPolicy) -> DownloadPolicy {
        switch p {
        case .unmeteredOrCharging: return .unmeteredOrCharging
        case .unmeteredOnly: return .unmeteredOnly
        case .anyNetwork: return .anyNetwork
        default: return fallback
        }
    }
}
