package com.fleetml.sdk

import android.content.Context
import com.fleetml.sdk.internal.DeviceStats
import com.fleetml.sdk.internal.GrpcClient
import com.fleetml.sdk.internal.Logger
import com.fleetml.sdk.internal.ModelManager
import com.fleetml.sdk.internal.ModelStore
import com.fleetml.sdk.internal.TelemetryQueue
import com.fleetml.sdk.internal.TokenManager
import fleetml.device.v1.Device.HeartbeatRequest
import fleetml.device.v1.Device.RefreshTokenRequest
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
 * Initialize once, e.g. in `Application.onCreate()`:
 * ```kotlin
 * FleetML.init(
 *     context = this,
 *     config = FleetMLConfig(serverHost = "10.0.2.2", enrollmentKey = "..."),
 *     callback = object : FleetMLCallback {
 *         override fun onModelReady(model: ActiveModel) { /* load model.file */ }
 *     },
 * )
 * ```
 *
 * The SDK is offline-first: models activated in an earlier run are delivered
 * through [FleetMLCallback.onModelReady] (and [activeModels]) as soon as
 * [start] runs, before the server is reached; the server reconciles in the
 * background.
 */
object FleetML {
    @Volatile private var instance: FleetMLInstance? = null

    /**
     * Initialize the SDK. A second call while initialized is ignored; call
     * [stop] first to re-initialize (e.g. with a new config).
     *
     * @param validator checks a new model before it becomes active; defaults
     *   to [ModelValidators.default] (TFLite or ONNX Runtime when present).
     */
    @Synchronized
    fun init(
        context: Context,
        config: FleetMLConfig,
        callback: FleetMLCallback? = null,
        validator: ModelValidator = ModelValidators.default(),
    ) {
        if (instance != null) {
            Logger.w("Already initialized — ignoring duplicate init()")
            return
        }
        val inst = FleetMLInstance(context.applicationContext, config, callback, validator)
        instance = inst
        if (config.autoStart) inst.start()
    }

    /** Start the SDK (only needed if [FleetMLConfig.autoStart] is false). */
    fun start() {
        (instance ?: error("FleetML not initialized. Call FleetML.init() first.")).start()
    }

    /** Stop background work and release resources. [init] can be called again afterwards. */
    @Synchronized
    fun stop() {
        instance?.stop()
        instance = null
    }

    /** True between [init] and [stop]. */
    fun isInitialized(): Boolean = instance != null

    /** The models currently active on this device (validated, on disk). */
    fun activeModels(): List<ActiveModel> = instance?.models?.activeModels().orEmpty()

    /** The active model with this name, or null. */
    fun activeModel(modelName: String): ActiveModel? = activeModels().firstOrNull { it.modelName == modelName }

    /** The local file of a cached variant, or null. */
    fun getModelFile(variantId: String): File? = instance?.cache?.getModelFile(variantId)

    /** True if the device has registered with the server. */
    fun isRegistered(): Boolean = instance?.tokenManager?.isRegistered() ?: false

    /** The telemetry reporter (events are queued even before registration). */
    fun getTelemetryReporter(): TelemetryReporter =
        instance?.telemetryReporter ?: error("FleetML not initialized. Call FleetML.init() first.")

    /** Clear cached models and registration data, and stop. */
    @Synchronized
    fun reset() {
        instance?.reset()
        instance = null
    }
}

internal class FleetMLInstance(
    context: Context,
    val config: FleetMLConfig,
    private val callback: FleetMLCallback?,
    validator: ModelValidator,
) {
    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.IO)
    private val root = File(context.filesDir, "fleetml")
    private val grpcClient = GrpcClient(config)
    val tokenManager = TokenManager(context)
    val cache = ModelCache(File(root, "models"), config.maxCacheSizeMb).also { it.cleanupTemp() }
    private val registrar = DeviceRegistrar(grpcClient, tokenManager, config, context)
    private val stats = DeviceStats(context)

    val telemetryReporter = TelemetryReporter(
        queue = TelemetryQueue(File(root, "telemetry.queue")),
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

    val models = ModelManager(
        store = ModelStore(File(root, "models.json")),
        cache = cache,
        downloader = ModelDownloader(cache),
        validator = validator,
        telemetry = telemetryReporter,
        callback = { callback },
    )

    private val heartbeat = HeartbeatManager(
        api = object : DeviceApi {
            override suspend fun heartbeat(token: String, request: HeartbeatRequest) =
                grpcClient.authenticatedStub(token).heartbeat(request)
            override suspend fun refreshToken(token: String) =
                grpcClient.authenticatedStub(token).refreshToken(RefreshTokenRequest.getDefaultInstance())
        },
        tokens = tokenManager,
        models = models,
        telemetry = telemetryReporter,
        callback = { callback },
        initialIntervalSeconds = config.heartbeatIntervalSeconds,
        reRegister = { register() },
    )

    fun start() {
        telemetryReporter.start(scope)
        scope.launch {
            // Offline start: what was active last time is usable immediately.
            models.activeModels().forEach { callback?.onModelReady(it) }
            while (registrar.needsRegistration()) {
                try {
                    register()
                } catch (e: Exception) {
                    Logger.e("Registration failed: ${e.message}", e)
                    callback?.onConnectionStatusChanged(false)
                    delay(REGISTER_RETRY_MS)
                }
            }
            heartbeat.start(scope)
        }
    }

    private suspend fun register() {
        val result = registrar.register()
        heartbeat.setInterval(result.heartbeatIntervalSeconds.toLong())
        // The server's DeviceConfig wins over the local defaults.
        telemetryReporter.configure(result.telemetryBatchSize, result.telemetryFlushIntervalSeconds.toLong())
        callback?.onRegistered(result.deviceId)
        telemetryReporter.onReconnected()
    }

    fun stop() {
        telemetryReporter.stop()
        heartbeat.stop()
        scope.cancel()
        grpcClient.shutdown()
    }

    fun reset() {
        stop()
        tokenManager.clear()
        cache.clear()
        root.deleteRecursively()
    }

    private companion object {
        const val REGISTER_RETRY_MS = 10_000L
    }
}
