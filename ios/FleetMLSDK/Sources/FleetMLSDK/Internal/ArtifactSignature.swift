import CryptoKit
import Foundation

/// A project's artifact signing key, as the server sends it.
struct SigningKey: Codable, Equatable {
    let keyID: String
    let publicKey: Data
    /// End of a rotated-out key's grace period, Unix seconds; 0 for the current key.
    let notAfter: Int64
}

/// Checks a model's signature before it is downloaded (see "Signatures" in
/// docs/ARCHITECTURE.md; the cases in internal/artifact/testdata/vectors.json
/// are tested here too).
enum ArtifactSignature {
    static func message(modelID: String, version: String, sha256: String) -> Data {
        Data("fleetml-artifact-v1\n\(modelID)\n\(version)\n\(sha256.lowercased())".utf8)
    }

    /// Returns nil if the artifact may be used, else why not.
    static func verify(keys: [SigningKey], keyID: String, signature: Data,
                       modelID: String, version: String, sha256: String, now: Date) -> String? {
        if keys.isEmpty { return signature.isEmpty ? nil : "the artifact is signed but no signing keys are known" }
        if signature.isEmpty { return "the artifact is not signed" }
        guard let key = keys.first(where: { $0.keyID == keyID }) else { return "unknown signing key \"\(keyID)\"" }
        if key.notAfter > 0 && Int64(now.timeIntervalSince1970) >= key.notAfter {
            return "signing key \(keyID) was rotated out and its grace period has ended"
        }
        guard let pub = try? Curve25519.Signing.PublicKey(rawRepresentation: key.publicKey),
              pub.isValidSignature(signature, for: message(modelID: modelID, version: version, sha256: sha256)) else {
            return "the signature does not verify with key \(keyID)"
        }
        return nil
    }
}
