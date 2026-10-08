import Foundation

/// A bounded FIFO of telemetry events persisted to a file, so events survive
/// app termination and are kept while offline or before registration.
/// Records are length-prefixed protobuf; a torn last record is ignored.
final class TelemetryQueue {
    static let defaultMaxEvents = 10_000
    private let url: URL
    private let maxEvents: Int
    private let lock = NSLock()
    private var events: [Fleetml_Telemetry_V1_TelemetryEvent] = []
    private(set) var dropped = 0

    init(url: URL, maxEvents: Int = TelemetryQueue.defaultMaxEvents) {
        self.url = url
        self.maxEvents = maxEvents
        try? FileManager.default.createDirectory(at: url.deletingLastPathComponent(), withIntermediateDirectories: true)
        load()
    }

    var count: Int { lock.withLock { events.count } }

    func add(_ event: Fleetml_Telemetry_V1_TelemetryEvent) {
        lock.withLock {
            events.append(event)
            if events.count > maxEvents {
                let excess = events.count - maxEvents
                events.removeFirst(excess)
                dropped += excess
                persist()
            } else {
                append(event)
            }
        }
    }

    func peek(_ n: Int) -> [Fleetml_Telemetry_V1_TelemetryEvent] { lock.withLock { Array(events.prefix(n)) } }

    func remove(_ n: Int) {
        lock.withLock {
            events.removeFirst(min(n, events.count))
            persist()
        }
    }

    private static func record(_ e: Fleetml_Telemetry_V1_TelemetryEvent) -> Data {
        let body = (try? e.serializedData()) ?? Data()
        var length = UInt32(body.count).bigEndian
        return Data(bytes: &length, count: 4) + body
    }

    private func append(_ e: Fleetml_Telemetry_V1_TelemetryEvent) {
        if let handle = try? FileHandle(forWritingTo: url) {
            defer { try? handle.close() }
            handle.seekToEndOfFile()
            handle.write(Self.record(e))
        } else {
            try? Self.record(e).write(to: url)
        }
    }

    private func persist() {
        var data = Data()
        for e in events { data.append(Self.record(e)) }
        try? data.write(to: url, options: .atomic)
    }

    private func load() {
        guard let data = try? Data(contentsOf: url) else { return }
        var offset = 0
        while offset + 4 <= data.count {
            let length = Int(data.subdata(in: offset..<offset + 4).withUnsafeBytes { $0.load(as: UInt32.self).bigEndian })
            offset += 4
            guard length <= 64 * 1024, offset + length <= data.count,
                  let e = try? Fleetml_Telemetry_V1_TelemetryEvent(serializedBytes: data.subdata(in: offset..<offset + length)) else { break }
            events.append(e)
            offset += length
        }
        if events.count > maxEvents {
            dropped += events.count - maxEvents
            events.removeFirst(events.count - maxEvents)
        }
        persist()
    }
}
