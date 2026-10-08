import Foundation
import Security

/// Where the device identity and token are kept.
protocol TokenStore: AnyObject {
    var deviceID: String? { get }
    var token: String? { get }
    var tokenExpiresAt: Date? { get }
    var tokenIssuedAt: Date? { get }
    var registrationSignature: String { get }
    func saveRegistration(deviceID: String, token: String, expiresAtUnix: Int64, signature: String)
    func saveToken(_ token: String, expiresAtUnix: Int64)
    func clear()
}

extension TokenStore {
    var isRegistered: Bool { deviceID != nil && token != nil }
}

/// Stores credentials in the Keychain (this device only, after first unlock).
final class KeychainTokenStore: TokenStore {
    private struct Credentials: Codable {
        var deviceID: String
        var token: String
        var expiresAt: Date
        var issuedAt: Date
        var signature: String
    }

    private let service: String
    private let lock = NSLock()
    private var cached: Credentials?

    init(service: String = "com.fleetml.sdk.credentials") {
        self.service = service
        cached = read()
    }

    var deviceID: String? { lock.withLock { cached?.deviceID } }
    var token: String? { lock.withLock { cached?.token } }
    var tokenExpiresAt: Date? { lock.withLock { cached?.expiresAt } }
    var tokenIssuedAt: Date? { lock.withLock { cached?.issuedAt } }
    var registrationSignature: String { lock.withLock { cached?.signature ?? "" } }

    func saveRegistration(deviceID: String, token: String, expiresAtUnix: Int64, signature: String) {
        write(Credentials(deviceID: deviceID, token: token, expiresAt: Date(timeIntervalSince1970: TimeInterval(expiresAtUnix)),
                          issuedAt: Date(), signature: signature))
    }

    func saveToken(_ token: String, expiresAtUnix: Int64) {
        lock.withLock { () -> Credentials? in cached }.map {
            var c = $0
            c.token = token
            c.expiresAt = Date(timeIntervalSince1970: TimeInterval(expiresAtUnix))
            c.issuedAt = Date()
            write(c)
        }
    }

    func clear() {
        lock.withLock {
            cached = nil
            SecItemDelete(query() as CFDictionary)
        }
    }

    private func query() -> [String: Any] {
        [kSecClass as String: kSecClassGenericPassword, kSecAttrService as String: service, kSecAttrAccount as String: "device"]
    }

    private func read() -> Credentials? {
        var q = query()
        q[kSecReturnData as String] = true
        var out: AnyObject?
        guard SecItemCopyMatching(q as CFDictionary, &out) == errSecSuccess, let data = out as? Data else { return nil }
        return try? JSONDecoder().decode(Credentials.self, from: data)
    }

    private func write(_ c: Credentials) {
        lock.withLock {
            cached = c
            guard let data = try? JSONEncoder().encode(c) else { return }
            SecItemDelete(query() as CFDictionary)
            var q = query()
            q[kSecValueData as String] = data
            q[kSecAttrAccessible as String] = kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly
            SecItemAdd(q as CFDictionary, nil)
        }
    }
}

/// When to refresh the device token: the server accepts RefreshToken in the
/// last 7 days of a token's life; the SDK asks 6 days before expiry (a day of
/// clock skew still lands inside the window), or at half the lifetime for
/// short-lived tokens, so a 24h token refreshes every 12h.
enum TokenRefreshPolicy {
    static let refreshBeforeExpiry: TimeInterval = 6 * 86_400

    static func needsRefresh(now: Date, expiresAt: Date?, issuedAt: Date?) -> Bool {
        guard let expiresAt else { return false }
        var window = refreshBeforeExpiry
        if let issuedAt, issuedAt < expiresAt { window = min(window, expiresAt.timeIntervalSince(issuedAt) / 2) }
        return expiresAt.timeIntervalSince(now) <= window
    }
}
