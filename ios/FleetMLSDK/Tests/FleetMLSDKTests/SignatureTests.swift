import CryptoKit
import XCTest
@testable import FleetMLSDK

/// The cases every client agrees on: internal/artifact/testdata/vectors.json.
final class SignatureTests: XCTestCase {
    private struct Vector: Decodable {
        struct Key: Decodable { let key_id: String; let public_key: Data; let not_after: Int64? }
        let name: String
        let keys: [Key]?
        let key_id: String
        let signature: Data?
        let model_id: String
        let version: String
        let sha256: String
        let now: Int64
        let valid: Bool
    }

    func testAllClientsAgreeOnTheSignatureVectors() throws {
        let url = URL(fileURLWithPath: #filePath).deletingLastPathComponent()
            .appendingPathComponent("../../../../internal/artifact/testdata/vectors.json").standardized
        let vectors = try JSONDecoder().decode([Vector].self, from: Data(contentsOf: url))
        XCTAssertGreaterThanOrEqual(vectors.count, 10)
        for v in vectors {
            let keys = (v.keys ?? []).map { SigningKey(keyID: $0.key_id, publicKey: $0.public_key, notAfter: $0.not_after ?? 0) }
            let reason = ArtifactSignature.verify(keys: keys, keyID: v.key_id, signature: v.signature ?? Data(), modelID: v.model_id,
                                                  version: v.version, sha256: v.sha256, now: Date(timeIntervalSince1970: TimeInterval(v.now)))
            XCTAssertEqual(reason == nil, v.valid, "\(v.name): \(reason ?? "valid")")
        }
    }

    func testOlderStoreFilesStillLoad() throws {
        let url = tempDir().appendingPathComponent("models.json")
        try Data(#"{"active":{"m1":{"modelID":"m1","modelName":"iris","version":"1.0.0","versionID":"v","variantID":"a","sha256":"x"}},"lastKnownGood":{},"failed":{}}"#.utf8).write(to: url)
        let store = ModelStore(url: url)
        XCTAssertEqual(store.active("m1")?.variantID, "a", "a store written before signing keys existed keeps its models")
        XCTAssertTrue(store.signingKeys.isEmpty)
    }
}
