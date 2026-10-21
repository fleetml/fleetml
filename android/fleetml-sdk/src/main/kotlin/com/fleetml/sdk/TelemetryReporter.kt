package com.fleetml.sdk

import com.fleetml.sdk.internal.Logger
import com.fleetml.sdk.internal.TelemetryQueue
import fleetml.telemetry.v1.Telemetry.FeatureHistogram
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

    /**
     * Report one inference by the given active model. [features] are the
     * model's input values by feature name: each configured feature (set on the
     * server, `PUT /models/{id}/features`) is binned into a histogram for
     * feature-level drift; other names are ignored.
     */
    fun reportInference(
        model: ActiveModel,
        latencyMs: Int,
        confidence: Float = 0f,
        predictionLabel: String = "",
        errorMessage: String = "",
        features: Map<String, FloatArray> = emptyMap(),
    ) = reportInference(model.versionId, latencyMs, confidence, predictionLabel, errorMessage = errorMessage,
        histograms = histograms(model.modelId, features))

    /** Report one inference for a model version ID. */
    fun reportInference(
        modelVersionId: String,
        latencyMs: Int,
        confidence: Float = 0f,
        predictionLabel: String = "",
        memoryUsageMb: Int? = null,
        batteryPercent: Float? = null,
        errorMessage: String = "",
    ) = reportInference(modelVersionId, latencyMs, confidence, predictionLabel, memoryUsageMb, batteryPercent, errorMessage, emptyList())

    private fun reportInference(
        modelVersionId: String,
        latencyMs: Int,
        confidence: Float,
        predictionLabel: String,
        memoryUsageMb: Int? = null,
        batteryPercent: Float? = null,
        errorMessage: String,
        histograms: List<FeatureHistogram>,
    ) {
        enqueue(
            event(modelVersionId, TelemetryEvent.EventType.INFERENCE, latencyMs, errorMessage)
                .setConfidenceScore(confidence)
                .setPredictionLabel(predictionLabel)
                .setMemoryUsageMb(memoryUsageMb ?: availableMemoryMb())
                .setBatteryPercent(batteryPercent ?: batteryPercent())
                .addAllFeatures(histograms)
                .build()
        )
    }

    /** Bin edges per model ID and feature name, from the server's DeviceConfig. */
    @Volatile private var featureBins: Map<String, Map<String, DoubleArray>> = emptyMap()

    internal fun setFeatureBins(bins: List<fleetml.device.v1.Device.FeatureBins>) {
        featureBins = bins.groupBy { it.modelId }.mapValues { (_, l) -> l.associate { it.feature to it.edgesList.toDoubleArray() } }
    }

    internal fun histograms(modelId: String, features: Map<String, FloatArray>): List<FeatureHistogram> {
        val bins = featureBins[modelId] ?: return emptyList()
        return features.mapNotNull { (name, values) ->
            val edges = bins[name] ?: return@mapNotNull null
            FeatureHistogram.newBuilder().setName(name).addAllCounts(bin(edges, values)).build()
        }
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
            .setEventId(java.util.UUID.randomUUID().toString()) // stored once, even if the batch is sent again
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
                Logger.failure("Telemetry flush failed, ${queue.size} events kept", e)
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

/** Counts values into edges.size + 1 bins (below the first edge, between edges, at or above the last); NaN is skipped. */
internal fun bin(edges: DoubleArray, values: FloatArray): List<Int> {
    val counts = IntArray(edges.size + 1)
    for (v in values) {
        if (v.isNaN()) continue
        var i = 0
        while (i < edges.size && edges[i] <= v) i++
        counts[i]++
    }
    return counts.toList()
}
