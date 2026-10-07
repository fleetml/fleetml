package com.fleetml.sdk

import com.fleetml.sdk.internal.Logger
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import okhttp3.OkHttpClient
import okhttp3.Request
import java.io.File
import java.util.concurrent.TimeUnit

/**
 * Downloads model files from presigned URLs and stores them in the local cache.
 */
internal class ModelDownloader(
    private val cache: ModelCache,
    private val config: FleetMLConfig
) {
    private val httpClient = OkHttpClient.Builder()
        .connectTimeout(30, TimeUnit.SECONDS)
        .readTimeout(120, TimeUnit.SECONDS)
        .build()

    /**
     * Download a model variant from a presigned URL.
     *
     * Checks cache first. If the model is already cached with matching SHA256,
     * returns immediately without downloading.
     */
    suspend fun download(
        variantId: String,
        downloadUrl: String,
        blobSha256: String
    ): DownloadResult {
        // Check cache first
        if (cache.isCached(variantId, blobSha256)) {
            val file = cache.getModelFile(variantId)
            if (file != null) {
                Logger.d("Model $variantId found in cache")
                return DownloadResult.Cached(file)
            }
        }

        Logger.d("Downloading model $variantId")
        return withContext(Dispatchers.IO) {
            try {
                val request = Request.Builder().url(downloadUrl).build()
                val response = httpClient.newCall(request).execute()

                if (!response.isSuccessful) {
                    return@withContext DownloadResult.Failed("HTTP ${response.code}")
                }

                val bytes = response.body?.bytes()
                    ?: return@withContext DownloadResult.Failed("Empty response body")

                val file = cache.save(variantId, bytes, blobSha256)
                if (file != null) {
                    Logger.i("Downloaded model $variantId (${bytes.size / 1024}KB)")
                    DownloadResult.Downloaded(file)
                } else {
                    DownloadResult.Failed("SHA256 verification failed")
                }
            } catch (e: Exception) {
                Logger.e("Download failed for $variantId", e)
                DownloadResult.Failed(e.message ?: "Unknown error")
            }
        }
    }
}

/** Result of a model download attempt. */
sealed class DownloadResult {
    /** Model was freshly downloaded and saved to disk. */
    data class Downloaded(val file: File) : DownloadResult()
    /** Model was already in cache with matching SHA256. */
    data class Cached(val file: File) : DownloadResult()
    /** Download or verification failed. */
    data class Failed(val reason: String) : DownloadResult()
}
