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

    private fun sha256(data: ByteArray): String {
        val digest = MessageDigest.getInstance("SHA-256")
        return digest.digest(data).joinToString("") { "%02x".format(it) }
    }
}
