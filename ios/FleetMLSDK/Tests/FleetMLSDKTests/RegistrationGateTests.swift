import XCTest
@testable import FleetMLSDK

final class RegistrationGateTests: XCTestCase {
    func testConcurrentCallersShareOneRegistration() async throws {
        let gate = RegistrationGate()
        let calls = Counter()
        try await withThrowingTaskGroup(of: Void.self) { group in
            for _ in 0..<3 {
                group.addTask {
                    try await gate.register {
                        calls.increment()
                        try await Task.sleep(nanoseconds: 100_000_000) // the Register call
                    }
                }
            }
            try await group.waitForAll()
        }
        XCTAssertEqual(calls.value, 1)
    }

    func testALaterCallRegistersAgain() async throws {
        let gate = RegistrationGate()
        let calls = Counter()
        try await gate.register { calls.increment() }
        try await gate.register { calls.increment() }
        XCTAssertEqual(calls.value, 2)
    }

    func testAFailureReachesEveryWaitingCaller() async {
        let gate = RegistrationGate()
        do {
            try await gate.register { throw URLError(.notConnectedToInternet) }
            XCTFail("expected an error")
        } catch {
            XCTAssertEqual((error as? URLError)?.code, .notConnectedToInternet)
        }
    }
}

private final class Counter {
    private let lock = NSLock()
    private var count = 0
    var value: Int { lock.withLock { count } }
    func increment() { lock.withLock { count += 1 } }
}
