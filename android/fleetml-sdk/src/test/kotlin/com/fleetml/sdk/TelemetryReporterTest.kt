package com.fleetml.sdk

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class TelemetryReporterTest {

    @Test
    fun `FleetMLConfig has telemetry batch size`() {
        val config = FleetMLConfig(serverHost = "localhost", telemetryBatchSize = 100)
        assertEquals(100, config.telemetryBatchSize)
    }

    @Test
    fun `FleetMLConfig default telemetry batch size is 50`() {
        val config = FleetMLConfig(serverHost = "localhost")
        assertEquals(50, config.telemetryBatchSize)
    }

    @Test
    fun `DownloadResult types work correctly`() {
        val downloaded = DownloadResult.Downloaded(java.io.File("/tmp/test"))
        val cached = DownloadResult.Cached(java.io.File("/tmp/cached"))
        val failed = DownloadResult.Failed("error")

        assertTrue(downloaded is DownloadResult.Downloaded)
        assertTrue(cached is DownloadResult.Cached)
        assertTrue(failed is DownloadResult.Failed)
        assertEquals("error", failed.reason)
    }

    @Test
    fun `RegisterResult stores all fields`() {
        val result = RegisterResult(
            deviceId = "dev-1",
            heartbeatIntervalSeconds = 30,
            telemetryBatchSize = 50,
            maxCacheSizeMb = 512
        )
        assertEquals("dev-1", result.deviceId)
        assertEquals(30, result.heartbeatIntervalSeconds)
    }
}
