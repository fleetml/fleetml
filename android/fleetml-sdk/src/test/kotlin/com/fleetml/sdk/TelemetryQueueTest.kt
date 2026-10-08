package com.fleetml.sdk

import com.fleetml.sdk.internal.TelemetryQueue
import fleetml.telemetry.v1.Telemetry.TelemetryEvent
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Before
import org.junit.Test
import java.io.File

class TelemetryQueueTest {
    private lateinit var dir: File
    private lateinit var file: File

    @Before
    fun setUp() {
        dir = File(System.getProperty("java.io.tmpdir"), "fleetml-queue-${System.nanoTime()}")
        file = File(dir, "telemetry.queue")
    }

    @After
    fun tearDown() {
        dir.deleteRecursively()
    }

    private fun ev(n: Int) = TelemetryEvent.newBuilder().setModelVersionId("v").setLatencyMs(n).build()

    @Test
    fun `events survive a new process in order`() {
        val q = TelemetryQueue(file)
        (1..5).forEach { q.add(ev(it)) }
        q.remove(2)

        val reopened = TelemetryQueue(file) // simulates process death and restart
        assertEquals(listOf(3, 4, 5), reopened.peek(10).map { it.latencyMs })
    }

    @Test
    fun `peek does not remove and remove takes the oldest`() {
        val q = TelemetryQueue(file)
        (1..4).forEach { q.add(ev(it)) }
        assertEquals(listOf(1, 2), q.peek(2).map { it.latencyMs })
        assertEquals(4, q.size)
        q.remove(3)
        assertEquals(listOf(4), q.peek(10).map { it.latencyMs })
        q.remove(10)
        assertEquals(0, TelemetryQueue(file).size)
    }

    @Test
    fun `bounded queue drops the oldest and counts drops`() {
        val q = TelemetryQueue(file, maxEvents = 3)
        (1..5).forEach { q.add(ev(it)) }
        assertEquals(listOf(3, 4, 5), q.peek(10).map { it.latencyMs })
        assertEquals(2, q.dropped)
        assertEquals(listOf(3, 4, 5), TelemetryQueue(file, maxEvents = 3).peek(10).map { it.latencyMs })
    }

    @Test
    fun `a truncated last record is ignored`() {
        val q = TelemetryQueue(file)
        q.add(ev(1))
        q.add(ev(2))
        file.appendBytes(byteArrayOf(0, 0, 0, 50, 1, 2)) // length 50 but only 2 bytes: torn write
        assertEquals(listOf(1, 2), TelemetryQueue(file).peek(10).map { it.latencyMs })
    }
}
