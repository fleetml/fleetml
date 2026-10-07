package com.fleetml.sdk

import android.app.ActivityManager
import android.content.Context
import android.content.Intent
import android.content.IntentFilter
import android.os.BatteryManager
import com.fleetml.sdk.internal.GrpcClient
import com.fleetml.sdk.internal.Logger
import com.fleetml.sdk.internal.TokenManager
import fleetml.telemetry.v1.Telemetry.TelemetryBatch
import fleetml.telemetry.v1.Telemetry.TelemetryEvent
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch
import java.util.concurrent.ConcurrentLinkedQueue

/**
 * Reports inference telemetry to the FleetML server.
 *
 * Events are buffered in memory and sent in batches for efficiency.
 *
 * Usage:
 * ```kotlin
 * val reporter = FleetML.getTelemetryReporter()
 * reporter.reportInference(
 *     modelVersionId = "...",
 *     latencyMs = 45,
 *     confidence = 0.92f,
 *     predictionLabel = "legitimate"
 * )
 * ```
 */
class TelemetryReporter internal constructor(
    private val grpcClient: GrpcClient,
    private val tokenManager: TokenManager,
    private val config: FleetMLConfig,
    private val context: Context
) {
    private val buffer = ConcurrentLinkedQueue<TelemetryEvent>()
    private var flushJob: Job? = null
    private val batchSize = config.telemetryBatchSize

    /**
     * Report a single inference event.
     * Events are buffered and sent in batches.
     */
    fun reportInference(
        modelVersionId: String,
        latencyMs: Int,
        confidence: Float = 0f,
        predictionLabel: String = "",
        memoryUsageMb: Int? = null,
        batteryPercent: Float? = null,
        errorMessage: String = ""
    ) {
        val event = TelemetryEvent.newBuilder()
            .setModelVersionId(modelVersionId)
            .setType(TelemetryEvent.EventType.INFERENCE)
            .setLatencyMs(latencyMs)
            .setConfidenceScore(confidence)
            .setPredictionLabel(predictionLabel)
            .setMemoryUsageMb(memoryUsageMb ?: getAvailableMemoryMB())
            .setBatteryPercent(batteryPercent ?: getBatteryPercent())
            .setErrorMessage(errorMessage)
            .setTimestampUnixMs(System.currentTimeMillis())
            .build()

        buffer.add(event)

        if (buffer.size >= batchSize) {
            flush()
        }
    }

    /**
     * Report a model lifecycle event (load, download, error).
     */
    fun reportModelStatus(
        modelVersionId: String,
        type: TelemetryEvent.EventType,
        latencyMs: Int = 0,
        errorMessage: String = ""
    ) {
        val event = TelemetryEvent.newBuilder()
            .setModelVersionId(modelVersionId)
            .setType(type)
            .setLatencyMs(latencyMs)
            .setErrorMessage(errorMessage)
            .setTimestampUnixMs(System.currentTimeMillis())
            .build()

        buffer.add(event)
    }

    /** Start periodic flush (every 30 seconds). */
    fun start(scope: CoroutineScope) {
        flushJob = scope.launch {
            while (isActive) {
                delay(30_000)
                flush()
            }
        }
    }

    /** Stop periodic flushing and send remaining events. */
    fun stop() {
        flushJob?.cancel()
        flushJob = null
        flush()
    }

    /**
     * Get available RAM in MB using ActivityManager.
     */
    private fun getAvailableMemoryMB(): Int {
        return try {
            val am = context.getSystemService(Context.ACTIVITY_SERVICE) as ActivityManager
            val memInfo = ActivityManager.MemoryInfo()
            am.getMemoryInfo(memInfo)
            (memInfo.availMem / (1024 * 1024)).toInt()
        } catch (e: Exception) {
            0
        }
    }

    /**
     * Get current battery percentage using BatteryManager.
     */
    private fun getBatteryPercent(): Float {
        return try {
            val filter = IntentFilter(Intent.ACTION_BATTERY_CHANGED)
            val batteryStatus = context.registerReceiver(null, filter)
            if (batteryStatus != null) {
                val level = batteryStatus.getIntExtra(BatteryManager.EXTRA_LEVEL, -1)
                val scale = batteryStatus.getIntExtra(BatteryManager.EXTRA_SCALE, -1)
                if (level >= 0 && scale > 0) level.toFloat() / scale.toFloat() * 100f else 0f
            } else {
                0f
            }
        } catch (e: Exception) {
            0f
        }
    }

    /** Flush buffered events to the server. */
    private fun flush() {
        val events = mutableListOf<TelemetryEvent>()
        while (true) {
            val event = buffer.poll() ?: break
            events.add(event)
            if (events.size >= batchSize) break
        }

        if (events.isEmpty()) return

        val token = tokenManager.getAuthToken() ?: return
        val deviceId = tokenManager.getDeviceId() ?: return

        try {
            val batch = TelemetryBatch.newBuilder()
                .setDeviceId(deviceId)
                .addAllEvents(events)
                .build()

            // Use blocking stub with auth metadata in a background thread
            Thread {
                try {
                    val blockingStub = fleetml.telemetry.v1.TelemetryServiceGrpc.newBlockingStub(
                        grpcClient.channel
                    ).withCallCredentials(com.fleetml.sdk.internal.BearerToken(token))
                    val ack = blockingStub.reportBatch(batch)
                    Logger.d("Telemetry flushed: accepted=${ack.acceptedCount} rejected=${ack.rejectedCount}")
                } catch (e: Exception) {
                    Logger.w("Telemetry flush failed: ${e.message}")
                    events.forEach { buffer.add(it) }
                }
            }.start()
        } catch (e: Exception) {
            Logger.w("Telemetry flush error: ${e.message}")
            events.forEach { buffer.add(it) }
        }
    }
}
