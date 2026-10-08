import CryptoKit
import Foundation

/// A stable SHA-256 fingerprint of this app on this hardware: the bundle
/// identifier, hardware model, CPU count and physical memory. No
/// user-resettable or privacy-sensitive identifiers (identifierForVendor,
/// advertising ID) are used, and two apps on one device are separate devices.
enum DeviceFingerprint {
    static func generate(bundleID: String = Bundle.main.bundleIdentifier ?? "unknown") -> String {
        let parts = [bundleID, hardwareModel(), String(ProcessInfo.processInfo.processorCount),
                     String(ProcessInfo.processInfo.physicalMemory / (1024 * 1024))]
        return SHA256.hash(data: Data(parts.joined(separator: ":").utf8)).map { String(format: "%02x", $0) }.joined()
    }

    static func hardwareModel() -> String {
        if let simulated = ProcessInfo.processInfo.environment["SIMULATOR_MODEL_IDENTIFIER"] { return simulated }
        var info = utsname()
        uname(&info)
        return withUnsafeBytes(of: &info.machine) { raw in
            String(decoding: raw.prefix(while: { $0 != 0 }), as: UTF8.self)
        }
    }

    static var osVersion: String {
        let v = ProcessInfo.processInfo.operatingSystemVersion
        return "\(v.majorVersion).\(v.minorVersion).\(v.patchVersion)"
    }

    static var ramMB: UInt32 { UInt32(ProcessInfo.processInfo.physicalMemory / (1024 * 1024)) }
}
