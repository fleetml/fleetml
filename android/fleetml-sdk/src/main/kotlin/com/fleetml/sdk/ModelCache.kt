package com.fleetml.sdk

import com.fleetml.sdk.internal.Logger
import org.json.JSONObject
import java.io.File
import java.security.MessageDigest

/**
 * Manages on-disk caching of downloaded model files.
 *
 * Models are stored as `{variantId}.bin` with metadata in `{variantId}.meta.json`.
 * LRU eviction is performed when the cache exceeds [maxSizeMb].
 */
internal class ModelCache(
    private val cacheDir: File,
    private val maxSizeMb: Long
) {
    init {
        cacheDir.mkdirs()
    }

    /**
     * Check if a model variant is already cached with matching SHA256.
     */
    fun isCached(variantId: String, expectedSha256: String): Boolean {
        val file = modelFile(variantId)
        if (!file.exists()) return false
        val meta = readMetadata(variantId) ?: return false
        return meta.optString("sha256") == expectedSha256
    }

    /**
     * Get the File path for a cached model. Returns null if not cached.
     */
    fun getModelFile(variantId: String): File? {
        val file = modelFile(variantId)
        return if (file.exists()) file else null
    }

    /**
     * Save a downloaded model to cache.
     * Verifies SHA256 after write. Returns the File on success, null on failure.
     */
    fun save(variantId: String, data: ByteArray, expectedSha256: String): File? {
        evictIfNeeded(data.size.toLong())

        val tmpFile = File(cacheDir, "$variantId.tmp")
        val targetFile = modelFile(variantId)

        return try {
            tmpFile.writeBytes(data)

            // Verify SHA256
            val actualSha256 = sha256(data)
            if (actualSha256 != expectedSha256) {
                Logger.w("SHA256 mismatch for $variantId: expected=$expectedSha256, actual=$actualSha256")
                tmpFile.delete()
                return null
            }

            // Atomic rename
            if (!tmpFile.renameTo(targetFile)) {
                tmpFile.delete()
                return null
            }

            // Write metadata
            val meta = JSONObject().apply {
                put("sha256", expectedSha256)
                put("size", data.size)
                put("downloaded_at", System.currentTimeMillis())
            }
            metadataFile(variantId).writeText(meta.toString())

            Logger.d("Cached model $variantId (${data.size / 1024}KB)")
            targetFile
        } catch (e: Exception) {
            Logger.e("Failed to save model $variantId", e)
            tmpFile.delete()
            targetFile.delete()
            null
        }
    }

    /** Total cache size in bytes. */
    fun currentSizeBytes(): Long {
        return cacheDir.listFiles()?.sumOf { it.length() } ?: 0L
    }

    /** Evict oldest files until under maxSizeMb (accounting for incoming data). */
    fun evictIfNeeded(incomingBytes: Long = 0) {
        val maxBytes = maxSizeMb * 1024 * 1024
        var currentSize = currentSizeBytes() + incomingBytes

        if (currentSize <= maxBytes) return

        // Sort by last modified (oldest first)
        val files = cacheDir.listFiles()
            ?.filter { it.extension == "bin" }
            ?.sortedBy { it.lastModified() }
            ?: return

        for (file in files) {
            if (currentSize <= maxBytes) break
            val size = file.length()
            val variantId = file.nameWithoutExtension
            file.delete()
            metadataFile(variantId).delete()
            currentSize -= size
            Logger.d("Evicted cached model $variantId")
        }
    }

    /** Clear entire cache. */
    fun clear() {
        cacheDir.listFiles()?.forEach { it.delete() }
        Logger.d("Cache cleared")
    }

    private fun modelFile(variantId: String) = File(cacheDir, "$variantId.bin")
    private fun metadataFile(variantId: String) = File(cacheDir, "$variantId.meta.json")

    private fun readMetadata(variantId: String): JSONObject? {
        val file = metadataFile(variantId)
        if (!file.exists()) return null
        return try {
            JSONObject(file.readText())
        } catch (_: Exception) {
            null
        }
    }

    private fun sha256(data: ByteArray): String {
        val digest = MessageDigest.getInstance("SHA-256")
        val hash = digest.digest(data)
        return hash.joinToString("") { "%02x".format(it) }
    }
}
