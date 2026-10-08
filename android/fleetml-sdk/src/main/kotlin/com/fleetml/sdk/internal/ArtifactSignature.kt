package com.fleetml.sdk.internal

import org.bouncycastle.math.ec.rfc8032.Ed25519
import java.util.Locale

/** A project's artifact signing key, as the server sends it. */
internal data class SigningKey(
    val keyId: String,
    val publicKey: ByteArray,
    /** End of a rotated-out key's grace period, Unix seconds; 0 for the current key. */
    val notAfter: Long,
) {
    override fun equals(other: Any?) =
        other is SigningKey && keyId == other.keyId && publicKey.contentEquals(other.publicKey) && notAfter == other.notAfter

    override fun hashCode() = keyId.hashCode()
}

/**
 * Checks a model's signature before it is downloaded (see "Signatures" in
 * docs/ARCHITECTURE.md; the cases in internal/artifact/testdata/vectors.json
 * are tested here too). ed25519 comes from BouncyCastle's lightweight API: the
 * platform has it only from API 33.
 */
internal object ArtifactSignature {
    fun message(modelId: String, version: String, sha256: String): ByteArray =
        "fleetml-artifact-v1\n$modelId\n$version\n${sha256.lowercase(Locale.ROOT)}".toByteArray(Charsets.UTF_8)

    /** Returns null if the artifact may be used, else why not. */
    fun verify(
        keys: List<SigningKey>, keyId: String, signature: ByteArray,
        modelId: String, version: String, sha256: String, nowSeconds: Long,
    ): String? {
        if (keys.isEmpty()) return if (signature.isEmpty()) null else "the artifact is signed but no signing keys are known"
        if (signature.isEmpty()) return "the artifact is not signed"
        val key = keys.firstOrNull { it.keyId == keyId } ?: return "unknown signing key \"$keyId\""
        if (key.notAfter > 0 && nowSeconds >= key.notAfter) {
            return "signing key $keyId was rotated out and its grace period has ended"
        }
        val msg = message(modelId, version, sha256)
        val ok = key.publicKey.size == Ed25519.PUBLIC_KEY_SIZE && signature.size == Ed25519.SIGNATURE_SIZE &&
            Ed25519.verify(signature, 0, key.publicKey, 0, msg, 0, msg.size)
        return if (ok) null else "the signature does not verify with key $keyId"
    }
}
