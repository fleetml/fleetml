package com.fleetml.sdk

import com.fleetml.sdk.internal.Logger
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import okhttp3.OkHttpClient
import okhttp3.Request
import java.io.File
import java.io.FileOutputStream
import java.io.IOException
import java.util.concurrent.TimeUnit

/**
 * Downloads model files from presigned URLs into the [ModelCache].
 *
 * The body is streamed to `{variantId}.part`. If the transfer is interrupted
 * the partial file is kept, and the next attempt asks for the rest with an
 * HTTP Range request (a server that ignores Range sends the whole file and
 * the download starts over). The sha256 of the complete file is checked
 * before it is renamed into place; a mismatch deletes it.
 */
internal class ModelDownloader(
    private val cache: ModelCache,
    private val httpClient: OkHttpClient = defaultClient(),
) {
    suspend fun download(
        variantId: String,
        downloadUrl: String,
        blobSha256: String,
        expectedSize: Long = 0,
        protected: Set<String> = emptySet(),
    ): DownloadResult {
        cache.getModelFile(variantId)?.takeIf { cache.isCached(variantId, blobSha256) }?.let {
            cache.touch(variantId)
            return DownloadResult.Cached(it)
        }
        return withContext(Dispatchers.IO) { fetch(variantId, downloadUrl, blobSha256, expectedSize, protected) }
    }

    private fun fetch(variantId: String, url: String, sha256: String, expectedSize: Long, protected: Set<String>): DownloadResult {
        val part = cache.partFile(variantId)
        var offset = if (part.exists()) part.length() else 0L
        if (expectedSize > 0 && offset > expectedSize) {
            part.delete()
            offset = 0
        }
        cache.evictIfNeeded((expectedSize - offset).coerceAtLeast(0), protected)

        val request = Request.Builder().url(url).apply {
            if (offset > 0) header("Range", "bytes=$offset-")
        }.build()

        try {
            httpClient.newCall(request).execute().use { response ->
                val append = when (response.code) {
                    206 -> true
                    200 -> false // full body: start over
                    416 -> return finish(variantId, part, sha256) // nothing left to fetch
                    else -> return DownloadResult.Failed("HTTP ${response.code}")
                }
                val body = response.body ?: return DownloadResult.Failed("empty response body")
                if (append) Logger.i("Resuming $variantId at byte $offset")
                FileOutputStream(part, append).use { out ->
                    body.byteStream().use { it.copyTo(out, 64 * 1024) }
                }
            }
        } catch (e: IOException) {
            Logger.w("Download of $variantId interrupted at ${part.length()} bytes: ${e.message}")
            return DownloadResult.Failed("interrupted after ${part.length()} bytes; the next attempt resumes")
        }
        return finish(variantId, part, sha256)
    }

    private fun finish(variantId: String, part: File, sha256: String): DownloadResult {
        val file = cache.commit(variantId, part, sha256) ?: return DownloadResult.Failed("SHA256 verification failed")
        Logger.i("Downloaded model $variantId (${file.length() / 1024}KB)")
        return DownloadResult.Downloaded(file)
    }

    companion object {
        fun defaultClient(): OkHttpClient = OkHttpClient.Builder()
            .connectTimeout(30, TimeUnit.SECONDS)
            .readTimeout(120, TimeUnit.SECONDS)
            .build()
    }
}

/** Result of a model download attempt. */
sealed class DownloadResult {
    /** Model was freshly downloaded, verified and saved to disk. */
    data class Downloaded(val file: File) : DownloadResult()

    /** Model was already in cache with matching SHA256. */
    data class Cached(val file: File) : DownloadResult()

    /** Download or verification failed. */
    data class Failed(val reason: String) : DownloadResult()
}
