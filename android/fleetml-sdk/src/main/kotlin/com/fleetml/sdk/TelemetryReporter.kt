package com.fleetml.sdk

import com.fleetml.sdk.internal.Logger
import com.fleetml.sdk.internal.TelemetryQueue
import fleetml.telemetry.v1.Telemetry.TelemetryEvent
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import java.util.concurrent.ConcurrentHashMap

/** Sends a batch of telemetry for a registered device; returns the accepted count. */
internal fun interface TelemetrySender {
    suspend fun send(deviceId: String, token: String, events: List<TelemetryEvent>): Int
}

/** The registered device's identity, or null before registration. */
internal data class DeviceCredentials(val deviceId: String, val token: String)

/**
 * Reports inference telemetry to the FleetML server.
 *
 * Events go into a persistent, bounded queue first ([TelemetryQueue]): they
 * survive process death and are kept while the device is offline or not yet
 * registered. A background loop sends them in batches every flush interval;
 * after a failed send it backs off exponentially (up to 5 minutes) and it
 * flushes immediately when the connection comes back.
 *
 * ```kotlin
 * FleetML.getTelemetryReporter().reportInference(model, latencyMs = 12, confidence = 0.93f, predictionLabel = "setosa")
 * ```
 */
class TelemetryReporter internal constructor(
    private val queue: TelemetryQueue,
    private val sender: TelemetrySender,
    private val credentials: () -> DeviceCredentials?,
    batchSize: Int,
    flushIntervalSeconds: Long,
    private val availableMemoryMb: () -> Int = { 0 },
    private val batteryPercent: () -> Float = { 0f },
    private val now: () -> Long = System::currentTimeMillis,
) {
    @Volatile private var batchSize = batchSize.coerceAtLeast(1)
    @Volatile private var flushIntervalMs = flushIntervalSeconds.coerceAtLeast(1) * 1000
    private val sendLock = Mutex()
    private val arms = ConcurrentHashMap<String, String>() // model version ID -> experiment arm ID
    private var loopJob: Job? = null
    private var scope: CoroutineScope? = null

    /** Number of events waiting to be sent. */
    val pendingCount: Int get() = queue.size

    /** Report one inference by the given active model. */
    fun reportInference(
        model: ActiveModel,
        latencyMs: Int,
        confidence: Float = 0f,
        predictionLabel: String = "",
        errorMessage: String = "",
    ) = reportInference(model.versionId, latencyMs, confidence, predictionLabel, errorMessage = errorMessage)

    /** Report one inference for a model version ID. */
    fun reportInference(
        modelVersionId: String,
        latencyMs: Int,
        confidence: Float = 0f,
        predictionLabel: String = "",
        memoryUsageMb: Int? = null,
        batteryPercent: Float? = null,
        errorMessage: String = "",
    ) {
        enqueue(
            event(modelVersionId, TelemetryEvent.EventType.INFERENCE, latencyMs, errorMessage)
                .setConfidenceScore(confidence)
                .setPredictionLabel(predictionLabel)
                .setMemoryUsageMb(memoryUsageMb ?: availableMemoryMb())
                .setBatteryPercent(batteryPercent ?: batteryPercent())
                .build()
        )
    }

    /** Report a model lifecycle event (MODEL_LOAD, MODEL_LOAD_FAILED, OOM, ERROR). */
    fun reportModelStatus(
        modelVersionId: String,
        type: TelemetryEvent.EventType,
        latencyMs: Int = 0,
        errorMessage: String = "",
    ) = enqueue(event(modelVersionId, type, latencyMs, errorMessage).build())

    private fun event(versionId: String, type: TelemetryEvent.EventType, latencyMs: Int, error: String) =
        TelemetryEvent.newBuilder()
            .setModelVersionId(versionId)
            .setExperimentArmId(arms[versionId].orEmpty())
            .setType(type)
            .setLatencyMs(latencyMs.coerceAtLeast(0))
            .setErrorMessage(error)
            .setTimestampUnixMs(now())

    private fun enqueue(event: TelemetryEvent) {
        queue.add(event)
        if (queue.size >= batchSize) scope?.launch { flushNow() }
    }

    /** Arms from the latest heartbeat: events for these versions are tagged with the arm ID. */
    internal fun setExperimentArms(versionToArm: Map<String, String>) {
        arms.clear()
        arms.putAll(versionToArm)
    }

    /** Apply the server's telemetry settings (DeviceConfig). */
    internal fun configure(batchSize: Int, flushIntervalSeconds: Long) {
        if (batchSize > 0) this.batchSize = batchSize
        if (flushIntervalSeconds > 0) this.flushIntervalMs = flushIntervalSeconds * 1000
    }

    /** Start the periodic flush loop. */
    internal fun start(scope: CoroutineScope) {
        this.scope = scope
        loopJob?.cancel()
        loopJob = scope.launch {
            var failures = 0
            while (isActive) {
                delay(if (failures == 0) flushIntervalMs else backoffMs(failures))
                failures = if (flushNow()) 0 else failures + 1
            }
        }
    }

    /** Called when the server is reachable again: send what queued up. */
    internal fun onReconnected() {
        scope?.launch { flushNow() }
    }

    internal fun stop() {
        loopJob?.cancel()
        loopJob = null
        scope = null
    }

    /**
     * Sends queued events in batches until the queue is empty. Returns false
     * if a send failed (the events stay queued). Unregistered devices keep
     * everything queued and return true.
     */
    internal suspend fun flushNow(): Boolean = sendLock.withLock {
        val creds = credentials() ?: return true
        var batch = queue.peek(batchSize)
        while (batch.isNotEmpty()) {
            try {
                val accepted = sender.send(creds.deviceId, creds.token, batch)
                queue.remove(batch.size)
                Logger.d("Telemetry flushed: ${batch.size} sent, $accepted accepted")
            } catch (e: Exception) {
                Logger.w("Telemetry flush failed, ${queue.size} events kept: ${e.message}")
                return false
            }
            batch = queue.peek(batchSize)
        }
        true
    }

    companion object {
        private const val BASE_BACKOFF_MS = 5_000L
        private const val MAX_BACKOFF_MS = 300_000L

        internal fun backoffMs(failures: Int): Long =
            (BASE_BACKOFF_MS shl (failures - 1).coerceIn(0, 10)).coerceAtMost(MAX_BACKOFF_MS)
    }
}
