package com.fleetml.sdk

import android.content.Context
import com.fleetml.sdk.internal.DeviceStats
import com.fleetml.sdk.internal.GrpcClient
import com.fleetml.sdk.internal.Logger
import com.fleetml.sdk.internal.TelemetryQueue
import com.fleetml.sdk.internal.TokenManager
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import java.io.File

/**
 * FleetML Android SDK entry point.
 *
 * Initialize once in your Application or Activity:
 * ```kotlin
 * FleetML.init(
 *     context = this,
 *     config = FleetMLConfig(serverHost = "10.0.2.2"),
 *     callback = object : FleetMLCallback {
 *         override fun onModelReady(modelName: String, version: String, variantId: String, modelFile: File) {
 *             // Load the TFLite model from modelFile
 *         }
 *         // ... other callbacks
 *     }
 * )
 * ```
 */
object FleetML {
    private var instance: FleetMLInstance? = null

    /**
     * Initialize FleetML SDK. Call once in Application.onCreate() or Activity.onCreate().
     * Subsequent calls are ignored with a warning.
     *
     * @param context Android context (application context is used internally)
     * @param config SDK configuration
     * @param callback Optional callback for lifecycle events
     */
    fun init(
        context: Context,
        config: FleetMLConfig,
        callback: FleetMLCallback? = null
    ) {
        if (instance != null) {
            Logger.w("Already initialized — ignoring duplicate init()")
            return
        }

        val appContext = context.applicationContext
        val grpcClient = GrpcClient(config)
        val tokenManager = TokenManager(appContext)
        val cacheDir = File(appContext.filesDir, "fleetml/models")
        val cache = ModelCache(cacheDir, config.maxCacheSizeMb)
        cache.cleanupTemp()
        val downloader = ModelDownloader(cache)
        val registrar = DeviceRegistrar(grpcClient, tokenManager, config, appContext)
        val heartbeatManager = HeartbeatManager(
            grpcClient, tokenManager, downloader, cache, callback, config,
            reRegister = { registrar.register() }
        )
        val stats = DeviceStats(appContext)
        val telemetryReporter = TelemetryReporter(
            queue = TelemetryQueue(File(appContext.filesDir, "fleetml/telemetry.queue")),
            sender = grpcClient.telemetrySender,
            credentials = {
                val id = tokenManager.getDeviceId()
                val token = tokenManager.getAuthToken()
                if (id != null && token != null) DeviceCredentials(id, token) else null
            },
            batchSize = config.telemetryBatchSize,
            flushIntervalSeconds = config.telemetryFlushIntervalSeconds,
            availableMemoryMb = stats::availableMemoryMb,
            batteryPercent = stats::batteryPercent,
        )

        val inst = FleetMLInstance(
            config = config,
            grpcClient = grpcClient,
            tokenManager = tokenManager,
            cache = cache,
            registrar = registrar,
            heartbeatManager = heartbeatManager,
            telemetryReporter = telemetryReporter,
            callback = callback
        )
        instance = inst

        if (config.autoStart) {
            inst.start()
        }
    }

    /**
     * Manually trigger registration + heartbeat start.
     * Only needed if [FleetMLConfig.autoStart] is false.
     */
    fun start() {
        instance?.start() ?: error("FleetML not initialized. Call FleetML.init() first.")
    }

    /** Stop heartbeat and release resources. */
    fun stop() {
        instance?.stop()
    }

    /**
     * Get the local File for a cached model variant.
     * Returns null if not yet downloaded.
     */
    fun getModelFile(variantId: String): File? {
        return instance?.cache?.getModelFile(variantId)
    }

    /** Check if the device is registered with the FleetML server. */
    fun isRegistered(): Boolean {
        return instance?.tokenManager?.isRegistered() ?: false
    }

    /**
     * Get the telemetry reporter for sending inference metrics.
     * @throws IllegalStateException if FleetML is not initialized
     */
    fun getTelemetryReporter(): TelemetryReporter {
        return instance?.telemetryReporter ?: error("FleetML not initialized. Call FleetML.init() first.")
    }

    /** Clear all cached models and registration data. */
    fun reset() {
        instance?.reset()
        instance = null
    }
}

internal class FleetMLInstance(
    val config: FleetMLConfig,
    val grpcClient: GrpcClient,
    val tokenManager: TokenManager,
    val cache: ModelCache,
    private val registrar: DeviceRegistrar,
    private val heartbeatManager: HeartbeatManager,
    val telemetryReporter: TelemetryReporter,
    private val callback: FleetMLCallback?
) {
    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.IO)

    fun start() {
        // Telemetry is queued from the start; it is sent once the device is registered.
        telemetryReporter.start(scope)
        scope.launch {
            try {
                val result = registrar.register()
                callback?.onRegistered(result.deviceId)
                telemetryReporter.configure(result.telemetryBatchSize, 0)
                telemetryReporter.onReconnected()
                heartbeatManager.start(scope)
            } catch (e: Exception) {
                Logger.e("Registration failed: ${e.message}", e)
                callback?.onConnectionStatusChanged(false)
                delay(10_000)
                start()
            }
        }
    }

    fun stop() {
        telemetryReporter.stop()
        heartbeatManager.stop()
        grpcClient.shutdown()
        scope.cancel()
    }

    fun reset() {
        stop()
        tokenManager.clear()
        cache.clear()
    }
}
