import Foundation

/// One registration at a time. Each Register call issues a new device token
/// that supersedes the previous one, so two concurrent registrations (start()
/// and syncNow() both finding the device unregistered) could leave the device
/// with the superseded token. A caller that arrives while a registration is
/// running waits for it and shares its result.
final class RegistrationGate {
    private let lock = NSLock()
    private var inFlight: Task<Void, Error>?

    func register(_ body: @escaping () async throws -> Void) async throws {
        let task: Task<Void, Error> = lock.withLock {
            if let running = inFlight { return running }
            let started = Task { try await body() }
            inFlight = started
            return started
        }
        defer { lock.withLock { if inFlight == task { inFlight = nil } } }
        try await task.value
    }
}
