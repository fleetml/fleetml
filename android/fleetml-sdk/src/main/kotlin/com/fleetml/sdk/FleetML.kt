package com.fleetml.sdk

import android.content.Context
import android.os.Handler
import android.os.Looper
import android.util.Log
import androidx.lifecycle.DefaultLifecycleObserver
import androidx.lifecycle.LifecycleOwner
import androidx.lifecycle.ProcessLifecycleOwner
import androidx.work.Constraints
import androidx.work.ExistingWorkPolicy
import androidx.work.NetworkType
import androidx.work.OneTimeWorkRequestBuilder
import androidx.work.WorkManager
import com.fleetml.sdk.internal.DeviceStats
import com.fleetml.sdk.internal.GrpcClient
import com.fleetml.sdk.internal.Logger
import com.fleetml.sdk.internal.ModelManager
import com.fleetml.sdk.internal.ModelStore
import com.fleetml.sdk.internal.RegistrationGate
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
import java.util.concurrent.TimeUnit

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
 *
 * Battery and data: heartbeats and telemetry flushes run on a timer only while
 * the app is in the foreground. In the background a WorkManager job (at most
 * every 15 minutes, by default hourly, on any connected network) runs one
 * heartbeat and one telemetry flush. Models download only as the
 * [DownloadPolicy] allows, unless the deployment is urgent.
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

    /**
     * Runs one heartbeat and one telemetry flush now, whatever the app's
     * state. The background job calls it; apps can too (e.g. pull to refresh).
     * A call while a heartbeat is in flight, such as at start-up, joins that
     * heartbeat, so a model is installed and reported to the callback once.
     * Returns true if the server was reached. A no-op before [init].
     */
    suspend fun syncNow(): Boolean = instance?.syncNow() ?: false

    /** Why the last [syncNow] did not reach the server or was refused by it; null after a success. */
    internal fun lastSyncFailure(): Throwable? = instance?.lastSyncFailure

    /** Schedules the next background check (called by the background job). */
    internal fun scheduleBackgroundSync() {
        instance?.scheduleBackgroundWork()
    }

    /** True between [init] and [stop]. */
    fun isInitialized(): Boolean = instance != null

    /** The models currently active on this device (validated, on disk). */
    fun activeModels(): List<ActiveModel> = instance?.models?.activeModels().orEmpty()

    /** The active model with this name, or null. */
    fun activeModel(modelName: String): ActiveModel? = activeModels().firstOrNull { it.modelName == modelName }

    /** The local file of a cached variant, or null. */
    fun getModelFile(variantId: String): File? = instance?.cache?.getModelFile(variantId)

    /** The device ID assigned by the server, or null before the first registration. */
    fun deviceId(): String? = instance?.tokenManager?.getDeviceId()

    /** True if the device has registered with the server. */
    fun isRegistered(): Boolean = instance?.tokenManager?.isRegistered() ?: false

    /** The telemetry reporter (events are queued even before registration). */
    fun getTelemetryReporter(): TelemetryReporter =
        instance?.telemetryReporter ?: error("FleetML not initialized. Call FleetML.init() first.")

    /**
     * Stop, and clear cached models and registration data. Pass a [context] to
     * clear them also when the SDK is not initialized (e.g. before [init]).
     */
    @Synchronized
    fun reset(context: Context? = null) {
        val inst = instance
        if (inst != null) inst.reset() else if (context != null) FleetMLInstance.clearStorage(context.applicationContext)
        instance = null
    }
}

internal class FleetMLInstance(
    private val context: Context,
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
    private val registration = RegistrationGate { registrar.needsRegistration() }
    private val stats = DeviceStats(context)

    // The server's DeviceConfig overrides these local defaults.
    @Volatile private var downloadPolicy = config.downloadPolicy
    @Volatile private var backgroundIntervalSeconds = config.backgroundHeartbeatIntervalSeconds

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
        canDownload = { urgent ->
            BatteryPolicy.mayDownload(downloadPolicy, urgent, stats.isConnected(), stats.isNetworkMetered(), stats.isCharging())
        },
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
        reRegister = { registration.register(::register) },
        onConfig = ::applyConfig,
    )

    /** Foreground: timers run. Background: they stop and a WorkManager job takes over. */
    private val lifecycleObserver = object : DefaultLifecycleObserver {
        override fun onStart(owner: LifecycleOwner) = onForeground()
        override fun onStop(owner: LifecycleOwner) = onBackground()
    }
    @Volatile private var started = false
    @Volatile private var registered = false
    @Volatile private var foreground = false
    @Volatile internal var lastSyncFailure: Throwable? = null
        private set

    fun start() {
        started = true
        scope.launch {
            // Offline start: what was active last time is usable immediately.
            models.activeModels().forEach { callback?.onModelReady(it) }
            while (registrar.needsRegistration()) {
                try {
                    registration.registerIfNeeded(::register)
                } catch (e: Exception) {
                    Logger.failure("Registration failed", e, Log.ERROR)
                    callback?.onConnectionStatusChanged(false)
                    delay(REGISTER_RETRY_MS)
                }
            }
            registered = true
            if (foreground) startTimers()
        }
        Handler(Looper.getMainLooper()).post { ProcessLifecycleOwner.get().lifecycle.addObserver(lifecycleObserver) }
    }

    private fun onForeground() {
        foreground = true
        WorkManager.getInstance(context).cancelUniqueWork(BACKGROUND_WORK)
        if (registered) startTimers()
    }

    private fun onBackground() {
        foreground = false
        // A sync the move cuts short (a download or a validation) is finished
        // by the background job at once, not after the background interval.
        val interrupted = heartbeat.inFlightNow
        heartbeat.stop()
        telemetryReporter.stop()
        scope.launch { telemetryReporter.flushNow() }
        scheduleBackgroundWork(interrupted)
    }

    private fun startTimers() {
        telemetryReporter.start(scope)
        heartbeat.start(scope)
    }

    /**
     * Schedules the next background check. It is a one-time job that the
     * worker re-schedules after each run, so the interval follows the server's
     * DeviceConfig without re-enqueueing a periodic job.
     */
    fun scheduleBackgroundWork(interrupted: Boolean = false) {
        if (foreground) return
        val interval = BatteryPolicy.backgroundDelaySeconds(backgroundIntervalSeconds, interrupted)
        val request = OneTimeWorkRequestBuilder<FleetMLSyncWorker>()
            .setInitialDelay(interval, TimeUnit.SECONDS)
            .setConstraints(Constraints.Builder().setRequiredNetworkType(NetworkType.CONNECTED).build())
            .build()
        WorkManager.getInstance(context).enqueueUniqueWork(BACKGROUND_WORK, ExistingWorkPolicy.REPLACE, request)
        Logger.i(if (interrupted) "In the background: finishing the sync through WorkManager" else "In the background: next sync in ${interval}s through WorkManager")
    }

    /** One heartbeat and one telemetry flush (the background job and [FleetML.syncNow]). */
    suspend fun syncNow(): Boolean {
        if (registrar.needsRegistration()) {
            try {
                registration.registerIfNeeded(::register)
                registered = true
            } catch (e: Exception) {
                lastSyncFailure = e
                Logger.failure("Registration failed", e)
                return false
            }
        }
        val reached = heartbeat.beatOnce()
        lastSyncFailure = if (reached) null else heartbeat.lastFailure
        telemetryReporter.flushNow()
        return reached
    }

    private suspend fun register() {
        val result = registrar.register()
        models.setSigningKeys(result.signingKeys)
        heartbeat.setInterval(result.heartbeatIntervalSeconds.toLong())
        // The server's DeviceConfig wins over the local defaults.
        applyConfig(result.config)
        callback?.onRegistered(result.deviceId)
        telemetryReporter.onReconnected()
    }

    private fun applyConfig(c: fleetml.device.v1.Device.DeviceConfig) {
        if (c.heartbeatIntervalSeconds > 0) heartbeat.setInterval(c.heartbeatIntervalSeconds.toLong())
        telemetryReporter.configure(c.telemetryBatchSize.toInt(), c.telemetryFlushIntervalSeconds.toLong())
        telemetryReporter.setFeatureBins(c.featureBinsList)
        downloadPolicy = BatteryPolicy.fromProto(c.downloadPolicy, downloadPolicy)
        if (c.backgroundHeartbeatIntervalSeconds > 0) backgroundIntervalSeconds = c.backgroundHeartbeatIntervalSeconds.toLong()
    }

    fun stop() {
        if (started) {
            Handler(Looper.getMainLooper()).post { ProcessLifecycleOwner.get().lifecycle.removeObserver(lifecycleObserver) }
            WorkManager.getInstance(context).cancelUniqueWork(BACKGROUND_WORK)
        }
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


    companion object {
        private const val REGISTER_RETRY_MS = 10_000L
        private const val BACKGROUND_WORK = "fleetml-background-sync"

        /** Deletes the registration and everything the SDK stored, without an instance. */
        fun clearStorage(context: Context) {
            TokenManager(context).clear()
            File(context.filesDir, "fleetml").deleteRecursively()
        }
    }
}
