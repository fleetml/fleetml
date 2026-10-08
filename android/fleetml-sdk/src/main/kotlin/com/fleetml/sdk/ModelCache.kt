package com.fleetml.sdk

import com.fleetml.sdk.internal.Logger
import org.json.JSONObject
import java.io.File
import java.security.MessageDigest

/**
 * On-disk store of downloaded model files.
 *
 * A verified model is `{variantId}.bin` with `{variantId}.meta.json`
 * (sha256, size). Downloads in progress are `{variantId}.part` so they can be
 * resumed. When the cache would exceed [maxSizeMb], least recently used
 * models are evicted, except the variants the caller protects (the active
 * and last-known-good models).
 */
internal class ModelCache(
    private val cacheDir: File,
    private val maxSizeMb: Long,
) {
    init {
        cacheDir.mkdirs()
    }

    /** True if the variant is stored with the expected sha256. */
    fun isCached(variantId: String, expectedSha256: String): Boolean {
        if (!modelFile(variantId).exists()) return false
        return readMetadata(variantId)?.optString("sha256").equals(expectedSha256, ignoreCase = true)
    }

    /** The stored model file, or null. */
    fun getModelFile(variantId: String): File? = modelFile(variantId).takeIf { it.exists() }

    /** Where a download of this variant is written until it is verified. */
    fun partFile(variantId: String): File = File(cacheDir, "$variantId.part")

    /**
     * Verifies a completed download and moves it into place. Returns the model
     * file, or null (and deletes the part) if its sha256 does not match.
     */
    fun commit(variantId: String, part: File, expectedSha256: String): File? {
        val actual = sha256(part)
        if (!actual.equals(expectedSha256, ignoreCase = true)) {
            Logger.w("SHA256 mismatch for $variantId: expected=$expectedSha256, actual=$actual")
            part.delete()
            return null
        }
        val target = modelFile(variantId)
        target.delete()
        if (!part.renameTo(target)) {
            part.delete()
            return null
        }
        metadataFile(variantId).writeText(
            JSONObject().put("sha256", actual).put("size", target.length())
                .put("downloaded_at", System.currentTimeMillis()).toString()
        )
        return target
    }

    /** Marks a model as recently used (eviction is least-recently-used). */
    fun touch(variantId: String) {
        modelFile(variantId).setLastModified(System.currentTimeMillis())
    }

    /** Total size of models and partial downloads, in bytes. */
    fun currentSizeBytes(): Long = cacheDir.listFiles()?.sumOf { it.length() } ?: 0L

    /**
     * Evicts least recently used models until [incomingBytes] more fit.
     * Variants in [protected] are never evicted.
     */
    fun evictIfNeeded(incomingBytes: Long = 0, protected: Set<String> = emptySet()) {
        val maxBytes = maxSizeMb * 1024 * 1024
        var size = currentSizeBytes() + incomingBytes
        if (size <= maxBytes) return
        val candidates = cacheDir.listFiles()
            ?.filter { it.extension == "bin" && it.nameWithoutExtension !in protected }
            ?.sortedBy { it.lastModified() }
            ?: return
        for (file in candidates) {
            if (size <= maxBytes) break
            size -= file.length()
            delete(file.nameWithoutExtension)
            Logger.d("Evicted cached model ${file.nameWithoutExtension}")
        }
    }

    /** Removes a model and its metadata. */
    fun delete(variantId: String) {
        modelFile(variantId).delete()
        metadataFile(variantId).delete()
    }

    /**
     * Removes leftovers: temp files, and partial downloads older than
     * [maxPartAgeMs] (younger ones are kept so they can be resumed).
     */
    fun cleanupTemp(maxPartAgeMs: Long = DAY_MS, now: Long = System.currentTimeMillis()) {
        cacheDir.listFiles()?.forEach { f ->
            when {
                f.extension == "tmp" -> f.delete()
                f.extension == "part" && now - f.lastModified() > maxPartAgeMs -> f.delete()
            }
        }
    }

    /** Clear the entire cache. */
    fun clear() {
        cacheDir.listFiles()?.forEach { it.delete() }
    }

    private fun modelFile(variantId: String) = File(cacheDir, "$variantId.bin")
    private fun metadataFile(variantId: String) = File(cacheDir, "$variantId.meta.json")

    private fun readMetadata(variantId: String): JSONObject? = try {
        metadataFile(variantId).takeIf { it.exists() }?.let { JSONObject(it.readText()) }
    } catch (_: Exception) {
        null
    }

    companion object {
        private const val DAY_MS = 24L * 60 * 60 * 1000

        fun sha256(file: File): String {
            val digest = MessageDigest.getInstance("SHA-256")
            file.inputStream().use { input ->
                val buf = ByteArray(64 * 1024)
                while (true) {
                    val n = input.read(buf)
                    if (n < 0) break
                    digest.update(buf, 0, n)
                }
            }
            return digest.digest().joinToString("") { "%02x".format(it) }
        }
    }
}
