package com.fleetml.sdk

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test
import java.io.File
import java.security.MessageDigest

class HeartbeatManagerTest {

    @Test
    fun `DownloadResult Downloaded contains file`() {
        val file = File("/tmp/test.bin")
        val result = DownloadResult.Downloaded(file)
        assertEquals(file, result.file)
    }

    @Test
    fun `DownloadResult Cached contains file`() {
        val file = File("/tmp/cached.bin")
        val result = DownloadResult.Cached(file)
        assertEquals(file, result.file)
    }

    @Test
    fun `DownloadResult Failed contains reason`() {
        val result = DownloadResult.Failed("network error")
        assertEquals("network error", result.reason)
    }

    @Test
    fun `LoadedModelInfo stores model details`() {
        val info = LoadedModelInfo(
            modelId = "m1",
            modelName = "fraud-detector",
            version = "1.0.0",
            variantId = "v1"
        )
        assertEquals("fraud-detector", info.modelName)
        assertEquals("1.0.0", info.version)
    }

    @Test
    fun `RegisterResult stores registration details`() {
        val result = RegisterResult(
            deviceId = "d1",
            heartbeatIntervalSeconds = 60,
            telemetryBatchSize = 100,
            maxCacheSizeMb = 512
        )
        assertEquals("d1", result.deviceId)
        assertEquals(60, result.heartbeatIntervalSeconds)
    }

    @Test
    fun `ModelCache save returns null on sha mismatch`() {
        val dir = File(System.getProperty("java.io.tmpdir"), "hb-test-${System.nanoTime()}")
        dir.mkdirs()
        try {
            val cache = ModelCache(dir, 10)
            val data = "test".toByteArray()
            val result = cache.save("v1", data, "bad-sha")
            assertNull(result)
        } finally {
            dir.deleteRecursively()
        }
    }

    @Test
    fun `ModelCache caches correctly with valid sha`() {
        val dir = File(System.getProperty("java.io.tmpdir"), "hb-test2-${System.nanoTime()}")
        dir.mkdirs()
        try {
            val cache = ModelCache(dir, 10)
            val data = "model bytes".toByteArray()
            val sha = sha256(data)
            val file = cache.save("v2", data, sha)
            assertEquals(true, file != null)
            assertEquals(true, cache.isCached("v2", sha))
        } finally {
            dir.deleteRecursively()
        }
    }

    private fun sha256(data: ByteArray): String {
        val digest = MessageDigest.getInstance("SHA-256")
        return digest.digest(data).joinToString("") { "%02x".format(it) }
    }
}
