package com.fleetml.sdk

import com.fleetml.sdk.internal.TelemetryQueue
import fleetml.telemetry.v1.Telemetry.TelemetryEvent
import kotlinx.coroutines.test.runTest
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import java.io.File

class TelemetryReporterTest {
    private lateinit var dir: File
    private val sent = mutableListOf<List<TelemetryEvent>>()
    private var failSends = false
    private var creds: DeviceCredentials? = null

    private val sender = TelemetrySender { _, _, events ->
        if (failSends) throw RuntimeException("UNAVAILABLE: io exception")
        sent.add(events)
        events.size
    }

    @Before
    fun setUp() {
        dir = File(System.getProperty("java.io.tmpdir"), "fleetml-reporter-${System.nanoTime()}")
    }

    @After
    fun tearDown() {
        dir.deleteRecursively()
    }

    private fun reporter(batchSize: Int = 3) = TelemetryReporter(
        queue = TelemetryQueue(File(dir, "q")),
        sender = sender,
        credentials = { creds },
        batchSize = batchSize,
        flushIntervalSeconds = 30,
        now = { 1_000L },
    )

    @Test
    fun `events before registration are queued, not dropped`() = runTest {
        val r = reporter()
        repeat(5) { r.reportInference("v1", latencyMs = it) }
        assertTrue(r.flushNow())
        assertTrue("nothing is sent without credentials", sent.isEmpty())
        assertEquals(5, r.pendingCount)

        creds = DeviceCredentials("dev", "tok")
        assertTrue(r.flushNow())
        assertEquals(listOf(3, 2), sent.map { it.size })
        assertEquals("sent in order", (0..4).toList(), sent.flatten().map { it.latencyMs })
        assertEquals(0, r.pendingCount)
    }

    @Test
    fun `a failed send keeps every event for the next attempt`() = runTest {
        creds = DeviceCredentials("dev", "tok")
        val r = reporter()
        repeat(4) { r.reportInference("v1", latencyMs = it) }
        failSends = true
        assertFalse(r.flushNow())
        assertEquals(4, r.pendingCount)

        failSends = false
        assertTrue(r.flushNow())
        assertEquals(4, sent.flatten().size)
    }

    @Test
    fun `queued events survive a restart of the app`() = runTest {
        reporter().reportInference("v1", latencyMs = 7)
        creds = DeviceCredentials("dev", "tok")
        val afterRestart = reporter()
        assertEquals(1, afterRestart.pendingCount)
        assertTrue(afterRestart.flushNow())
        assertEquals(7, sent.single().single().latencyMs)
    }

    @Test
    fun `events are tagged with the device's experiment arm`() = runTest {
        creds = DeviceCredentials("dev", "tok")
        val r = reporter()
        r.setExperimentArms(mapOf("v-arm" to "arm-123"))
        r.reportInference("v-arm", latencyMs = 1)
        r.reportInference("v-other", latencyMs = 1)
        r.reportModelStatus("v-arm", TelemetryEvent.EventType.MODEL_LOAD_FAILED, errorMessage = "bad model")
        r.flushNow()
        val events = sent.flatten()
        assertEquals(listOf("arm-123", "", "arm-123"), events.map { it.experimentArmId })
        assertEquals("bad model", events[2].errorMessage)
        assertEquals(1_000L, events[0].timestampUnixMs)
    }

    @Test
    fun `backoff doubles up to five minutes`() {
        assertEquals(5_000L, TelemetryReporter.backoffMs(1))
        assertEquals(10_000L, TelemetryReporter.backoffMs(2))
        assertEquals(40_000L, TelemetryReporter.backoffMs(4))
        assertEquals(300_000L, TelemetryReporter.backoffMs(20))
    }

    @Test
    fun `FleetMLConfig telemetry defaults`() {
        val config = FleetMLConfig(serverHost = "localhost")
        assertEquals(50, config.telemetryBatchSize)
        assertEquals(30L, config.telemetryFlushIntervalSeconds)
    }
}
