import CryptoKit
import Foundation
import XCTest
@testable import FleetMLSDK

func tempDir() -> URL {
    let dir = FileManager.default.temporaryDirectory.appendingPathComponent("fleetml-tests-\(UUID().uuidString)", isDirectory: true)
    try? FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
    return dir
}

func sha256Hex(_ data: Data) -> String { SHA256.hash(data: data).map { String(format: "%02x", $0) }.joined() }

/// The sample Iris models in models/iris at the repository root.
func irisModel(_ name: String) -> URL {
    URL(fileURLWithPath: #filePath).deletingLastPathComponent() // FleetMLSDKTests
        .deletingLastPathComponent().deletingLastPathComponent() // FleetMLSDK
        .deletingLastPathComponent().deletingLastPathComponent() // repository root
        .appendingPathComponent("models/iris/\(name)")
}

/// Serves registered bodies by URL path, honouring `Range: bytes=N-`. A body
/// can be cut short once (`failAfter`) to simulate a dropped connection.
final class StubURLProtocol: URLProtocol {
    struct Route {
        var body: Data
        var status = 200
        var honourRange = true
        var failAfter: Int?
    }

    private static let lock = NSLock()
    private static var routes: [String: Route] = [:]
    private static var seen: [URLRequest] = []

    static func reset() { lock.withLock { routes = [:]; seen = [] } }
    static func set(_ path: String, _ route: Route) { lock.withLock { routes[path] = route } }
    static var requests: [URLRequest] { lock.withLock { seen } }

    static func configuration() -> URLSessionConfiguration {
        let config = URLSessionConfiguration.ephemeral
        config.protocolClasses = [StubURLProtocol.self]
        return config
    }

    override class func canInit(with request: URLRequest) -> Bool { true }
    override class func canonicalRequest(for request: URLRequest) -> URLRequest { request }
    override func stopLoading() {}

    override func startLoading() {
        let path = request.url?.path ?? ""
        let route: Route? = Self.lock.withLock {
            Self.seen.append(request)
            let r = Self.routes[path]
            if r?.failAfter != nil { Self.routes[path]?.failAfter = nil } // fail once
            return r
        }
        guard let route else {
            respond(status: 404, headers: [:], body: Data())
            return
        }
        var body = route.body
        var status = route.status
        var headers = ["Content-Length": String(body.count)]
        if route.honourRange, status == 200, let range = request.value(forHTTPHeaderField: "Range"),
           let start = Int(range.dropFirst("bytes=".count).dropLast()) {
            if start >= body.count {
                respond(status: 416, headers: [:], body: Data())
                return
            }
            body = body.subdata(in: start..<body.count)
            status = 206
            headers = ["Content-Length": String(body.count), "Content-Range": "bytes \(start)-\(route.body.count - 1)/\(route.body.count)"]
        }
        let response = HTTPURLResponse(url: request.url!, statusCode: status, httpVersion: "HTTP/1.1", headerFields: headers)!
        client?.urlProtocol(self, didReceive: response, cacheStoragePolicy: .notAllowed)
        if let n = route.failAfter {
            client?.urlProtocol(self, didLoad: body.prefix(n))
            // The connection drops a moment later, as on a real network.
            DispatchQueue.global().asyncAfter(deadline: .now() + 0.1) {
                self.client?.urlProtocol(self, didFailWithError: URLError(.networkConnectionLost))
            }
        } else {
            client?.urlProtocol(self, didLoad: body)
            client?.urlProtocolDidFinishLoading(self)
        }
    }

    private func respond(status: Int, headers: [String: String], body: Data) {
        let response = HTTPURLResponse(url: request.url!, statusCode: status, httpVersion: "HTTP/1.1", headerFields: headers)!
        client?.urlProtocol(self, didReceive: response, cacheStoragePolicy: .notAllowed)
        client?.urlProtocol(self, didLoad: body)
        client?.urlProtocolDidFinishLoading(self)
    }
}

final class RecordingDelegate: FleetMLDelegate {
    private let lock = NSLock()
    private var _ready: [ActiveModel] = []
    private var _failed: [(String, String, String)] = []
    private var _unloaded: [String] = []
    private var _connection: [Bool] = []
    private var _connectionFailures: [String] = []

    var ready: [ActiveModel] { lock.withLock { _ready } }
    var failed: [(name: String, version: String, reason: String)] { lock.withLock { _failed.map { ($0.0, $0.1, $0.2) } } }
    var unloaded: [String] { lock.withLock { _unloaded } }
    var connection: [Bool] { lock.withLock { _connection } }
    var connectionFailures: [String] { lock.withLock { _connectionFailures } }

    func fleetML(modelReady model: ActiveModel) { lock.withLock { _ready.append(model) } }
    func fleetML(modelLoadFailed modelName: String, version: String, reason: String) { lock.withLock { _failed.append((modelName, version, reason)) } }
    func fleetML(modelUnloaded modelID: String, modelName: String, variantID: String) { lock.withLock { _unloaded.append(variantID) } }
    func fleetML(connectionChanged connected: Bool) { lock.withLock { _connection.append(connected) } }
    func fleetML(connectionFailed error: Error) { lock.withLock { _connectionFailures.append(String(describing: error)) } }
}

final class FakeSender: TelemetrySender {
    private let lock = NSLock()
    private var _batches: [[Fleetml_Telemetry_V1_TelemetryEvent]] = []
    var failing = false

    var batches: [[Fleetml_Telemetry_V1_TelemetryEvent]] { lock.withLock { _batches } }
    var events: [Fleetml_Telemetry_V1_TelemetryEvent] { batches.flatMap { $0 } }

    func send(deviceID: String, token: String, events: [Fleetml_Telemetry_V1_TelemetryEvent]) async throws -> Int {
        if lock.withLock({ failing }) { throw URLError(.notConnectedToInternet) }
        lock.withLock { _batches.append(events) }
        return events.count
    }
}

final class MemoryTokenStore: TokenStore {
    var deviceID: String?
    var token: String?
    var tokenExpiresAt: Date?
    var tokenIssuedAt: Date?
    var registrationSignature = ""

    func saveRegistration(deviceID: String, token: String, expiresAtUnix: Int64, signature: String) {
        self.deviceID = deviceID
        self.token = token
        tokenExpiresAt = Date(timeIntervalSince1970: TimeInterval(expiresAtUnix))
        tokenIssuedAt = Date()
        registrationSignature = signature
    }

    func saveToken(_ token: String, expiresAtUnix: Int64) {
        self.token = token
        tokenExpiresAt = Date(timeIntervalSince1970: TimeInterval(expiresAtUnix))
        tokenIssuedAt = Date()
    }

    func clear() { deviceID = nil; token = nil; tokenExpiresAt = nil; tokenIssuedAt = nil; registrationSignature = "" }
}

func event(_ version: String, latency: UInt32 = 1) -> Fleetml_Telemetry_V1_TelemetryEvent {
    var e = Fleetml_Telemetry_V1_TelemetryEvent()
    e.modelVersionID = version
    e.type = .inference
    e.latencyMs = latency
    return e
}
