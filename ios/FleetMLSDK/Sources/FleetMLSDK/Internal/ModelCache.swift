import CryptoKit
import Foundation

/// On-disk store of downloaded models: `{variant}.bin` plus `{variant}.json`
/// (sha256). Downloads in progress are `{variant}.part` so they can resume.
/// Eviction is least recently used and never removes protected variants.
final class ModelCache {
    let directory: URL
    private let maxBytes: Int64
    private let fm = FileManager.default

    init(directory: URL, maxSizeMB: Int) {
        self.directory = directory
        self.maxBytes = Int64(maxSizeMB) * 1024 * 1024
        try? fm.createDirectory(at: directory, withIntermediateDirectories: true)
    }

    func modelFile(_ variant: String) -> URL { directory.appendingPathComponent("\(variant).bin") }
    func partFile(_ variant: String) -> URL { directory.appendingPathComponent("\(variant).part") }
    private func metaFile(_ variant: String) -> URL { directory.appendingPathComponent("\(variant).json") }

    func cachedFile(_ variant: String) -> URL? { fm.fileExists(atPath: modelFile(variant).path) ? modelFile(variant) : nil }

    func isCached(_ variant: String, sha256: String) -> Bool {
        guard cachedFile(variant) != nil, let data = try? Data(contentsOf: metaFile(variant)),
              let meta = try? JSONDecoder().decode([String: String].self, from: data) else { return false }
        return meta["sha256"]?.lowercased() == sha256.lowercased()
    }

    /// Verifies a completed download and moves it into place; nil (part deleted) on a hash mismatch.
    func commit(_ variant: String, part: URL, sha256 expected: String) -> URL? {
        guard let actual = try? Self.sha256(of: part), actual == expected.lowercased() else {
            try? fm.removeItem(at: part)
            return nil
        }
        let target = modelFile(variant)
        try? fm.removeItem(at: target)
        do {
            try fm.moveItem(at: part, to: target)
            try JSONEncoder().encode(["sha256": actual]).write(to: metaFile(variant))
            return target
        } catch {
            try? fm.removeItem(at: part)
            return nil
        }
    }

    func touch(_ variant: String) {
        try? fm.setAttributes([.modificationDate: Date()], ofItemAtPath: modelFile(variant).path)
    }

    func delete(_ variant: String) {
        try? fm.removeItem(at: modelFile(variant))
        try? fm.removeItem(at: metaFile(variant))
    }

    var currentSizeBytes: Int64 {
        let files = (try? fm.contentsOfDirectory(at: directory, includingPropertiesForKeys: [.fileSizeKey])) ?? []
        return files.reduce(0) { $0 + Int64((try? $1.resourceValues(forKeys: [.fileSizeKey]).fileSize) ?? 0) }
    }

    func evictIfNeeded(incoming: Int64 = 0, protected: Set<String> = []) {
        var size = currentSizeBytes + incoming
        guard size > maxBytes else { return }
        let keys: [URLResourceKey] = [.contentModificationDateKey, .fileSizeKey]
        let candidates = ((try? fm.contentsOfDirectory(at: directory, includingPropertiesForKeys: keys)) ?? [])
            .filter { $0.pathExtension == "bin" && !protected.contains($0.deletingPathExtension().lastPathComponent) }
            .sorted {
                let a = (try? $0.resourceValues(forKeys: Set(keys)).contentModificationDate) ?? .distantPast
                let b = (try? $1.resourceValues(forKeys: Set(keys)).contentModificationDate) ?? .distantPast
                return a < b
            }
        for file in candidates where size > maxBytes {
            size -= Int64((try? file.resourceValues(forKeys: [.fileSizeKey]).fileSize) ?? 0)
            delete(file.deletingPathExtension().lastPathComponent)
        }
    }

    /// Deletes partial downloads older than `maxPartAge` (younger ones can resume).
    func cleanupParts(maxPartAge: TimeInterval = 86_400, now: Date = Date()) {
        let files = (try? fm.contentsOfDirectory(at: directory, includingPropertiesForKeys: [.contentModificationDateKey])) ?? []
        for f in files where f.pathExtension == "part" {
            let modified = (try? f.resourceValues(forKeys: [.contentModificationDateKey]).contentModificationDate) ?? .distantPast
            if now.timeIntervalSince(modified) > maxPartAge { try? fm.removeItem(at: f) }
        }
    }

    func clear() { try? fm.removeItem(at: directory); try? fm.createDirectory(at: directory, withIntermediateDirectories: true) }

    static func sha256(of url: URL) throws -> String {
        let handle = try FileHandle(forReadingFrom: url)
        defer { try? handle.close() }
        var hasher = SHA256()
        while true {
            let chunk = handle.readData(ofLength: 64 * 1024)
            if chunk.isEmpty { break }
            hasher.update(data: chunk)
        }
        return hasher.finalize().map { String(format: "%02x", $0) }.joined()
    }
}

/// The outcome of a download attempt.
enum DownloadResult: Equatable {
    case downloaded(URL)
    case cached(URL)
    case failed(String)
}

/// Streams model files from presigned URLs into the cache. Each chunk is
/// written to a `.part` file as it arrives, so an interrupted transfer keeps
/// what it received and the next attempt sends an HTTP Range request for the
/// rest (a server that ignores Range sends everything and the download
/// restarts). The sha256 is checked before the file is moved into place.
final class ModelDownloader {
    private let cache: ModelCache
    private let configuration: URLSessionConfiguration

    init(cache: ModelCache, configuration: URLSessionConfiguration = .default) {
        self.cache = cache
        self.configuration = configuration
    }

    func download(variant: String, url: URL, sha256: String, expectedSize: Int64 = 0, protected: Set<String> = []) async -> DownloadResult {
        if let file = cache.cachedFile(variant), cache.isCached(variant, sha256: sha256) {
            cache.touch(variant)
            return .cached(file)
        }
        let part = cache.partFile(variant)
        var offset = Self.size(of: part)
        if expectedSize > 0 && offset > expectedSize {
            try? FileManager.default.removeItem(at: part)
            offset = 0
        }
        cache.evictIfNeeded(incoming: max(0, expectedSize - offset), protected: protected)

        var request = URLRequest(url: url)
        if offset > 0 { request.setValue("bytes=\(offset)-", forHTTPHeaderField: "Range") }
        let writer = PartWriter(part: part)
        let session = URLSession(configuration: configuration, delegate: writer, delegateQueue: nil)
        defer { session.finishTasksAndInvalidate() }
        let task = session.dataTask(with: request)
        let (status, error) = await withTaskCancellationHandler {
            await writer.run(task)
        } onCancel: {
            task.cancel()
        }

        switch status {
        case 200, 206:
            if error != nil { return .failed("interrupted after \(Self.size(of: part)) bytes; the next attempt resumes") }
            return finish(variant, part: part, sha256: sha256)
        case 416: return finish(variant, part: part, sha256: sha256) // the part is already complete
        case 0: return .failed(error?.localizedDescription ?? "no response")
        default: return .failed("HTTP \(status)")
        }
    }

    private func finish(_ variant: String, part: URL, sha256: String) -> DownloadResult {
        guard let file = cache.commit(variant, part: part, sha256: sha256) else { return .failed("SHA256 verification failed") }
        return .downloaded(file)
    }

    private static func size(of url: URL) -> Int64 {
        (try? FileManager.default.attributesOfItem(atPath: url.path)[.size] as? Int64) ?? 0
    }
}

/// Writes a data task's body to the part file: appended for 206, from the
/// start for 200. Delegate callbacks are serial (the session's own queue).
private final class PartWriter: NSObject, URLSessionDataDelegate {
    private let part: URL
    private var handle: FileHandle?
    private var status = 0
    private var continuation: CheckedContinuation<(Int, Error?), Never>?

    init(part: URL) { self.part = part }

    func run(_ task: URLSessionDataTask) async -> (Int, Error?) {
        await withCheckedContinuation { c in
            continuation = c
            task.resume()
        }
    }

    func urlSession(_ session: URLSession, dataTask: URLSessionDataTask, didReceive response: URLResponse,
                    completionHandler: @escaping (URLSession.ResponseDisposition) -> Void) {
        status = (response as? HTTPURLResponse)?.statusCode ?? 0
        guard status == 200 || status == 206 else { return completionHandler(.cancel) }
        let fm = FileManager.default
        if status == 200 || !fm.fileExists(atPath: part.path) { fm.createFile(atPath: part.path, contents: nil) }
        handle = try? FileHandle(forWritingTo: part)
        handle?.seekToEndOfFile()
        completionHandler(handle == nil ? .cancel : .allow)
    }

    func urlSession(_ session: URLSession, dataTask: URLSessionDataTask, didReceive data: Data) {
        handle?.write(data)
    }

    func urlSession(_ session: URLSession, task: URLSessionTask, didCompleteWithError error: Error?) {
        try? handle?.close()
        handle = nil
        continuation?.resume(returning: (status, error))
        continuation = nil
    }
}
