import Foundation

/// Applies a model delta made by the server (internal/delta in the server repository: bsdiff with
/// raw DEFLATE sections). The cases in internal/delta/testdata are tested here too.
enum ModelDelta {
    struct Corrupt: Error, CustomStringConvertible { let description: String }

    private static let magic = Data("FMLDLT01".utf8)
    private static let header = 8 + 3 * 8

    static func patch(old: Data, patch: Data) throws -> Data {
        let p = [UInt8](patch)
        guard p.count >= header, Data(p[0..<8]) == magic else { throw Corrupt(description: "not a FleetML delta") }
        func int64(_ bytes: ArraySlice<UInt8>) -> Int64 {
            bytes.reversed().reduce(Int64(0)) { ($0 << 8) | Int64($1) }
        }
        let newSize = int64(p[8..<16]), ctrlLen = int64(p[16..<24]), diffLen = int64(p[24..<32])
        guard newSize >= 0, newSize <= Int64(Int32.max), ctrlLen >= 0, diffLen >= 0, ctrlLen + diffLen <= Int64(p.count - header) else {
            throw Corrupt(description: "corrupt delta header")
        }
        func inflate(_ from: Int, _ to: Int) throws -> [UInt8] {
            if from == to { return [] }
            // NSData's .zlib is raw DEFLATE (RFC 1951), as the server writes it.
            guard let d = try? (Data(p[(header + from)..<(header + to)]) as NSData).decompressed(using: .zlib) else {
                throw Corrupt(description: "corrupt delta section")
            }
            return [UInt8](d as Data)
        }
        let c = Int(ctrlLen), d = Int(diffLen)
        let ctrl = try inflate(0, c), diff = try inflate(c, c + d), extra = try inflate(c + d, p.count - header)
        let oldBytes = [UInt8](old)

        var out = [UInt8](repeating: 0, count: Int(newSize))
        var oldPos = 0, newPos = 0, ci = 0, di = 0, ei = 0
        while newPos < out.count {
            guard ci + 24 <= ctrl.count else { throw Corrupt(description: "corrupt delta control") }
            let add = Int(int64(ctrl[ci..<ci + 8])), copy = Int(int64(ctrl[ci + 8..<ci + 16])), seek = Int(int64(ctrl[ci + 16..<ci + 24]))
            ci += 24
            guard add >= 0, copy >= 0, newPos + add + copy <= out.count, di + add <= diff.count, ei + copy <= extra.count else {
                throw Corrupt(description: "corrupt delta control")
            }
            for i in 0..<add {
                var b = diff[di + i]
                let o = oldPos + i
                if o >= 0 && o < oldBytes.count { b = b &+ oldBytes[o] }
                out[newPos + i] = b
            }
            di += add
            newPos += add
            oldPos += add
            out.replaceSubrange(newPos..<newPos + copy, with: extra[ei..<ei + copy])
            ei += copy
            newPos += copy
            oldPos += seek
        }
        return Data(out)
    }
}
