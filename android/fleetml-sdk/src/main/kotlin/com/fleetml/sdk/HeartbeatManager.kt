package com.fleetml.sdk

import com.fleetml.sdk.internal.Logger
import com.fleetml.sdk.internal.ModelManager
import com.fleetml.sdk.internal.TokenRefreshPolicy
import com.fleetml.sdk.internal.TokenStore
import fleetml.device.v1.Device.HeartbeatRequest
import fleetml.device.v1.Device.HeartbeatResponse
import fleetml.device.v1.Device.RefreshTokenResponse
import io.grpc.Status
import io.grpc.StatusException
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch

/** The authenticated device RPCs the heartbeat loop uses. */
internal interface DeviceApi {
    suspend fun heartbeat(token: String, request: HeartbeatRequest): HeartbeatResponse
    suspend fun refreshToken(token: String): RefreshTokenResponse
}

/**
 * The periodic heartbeat loop: refresh the token when it nears expiry,
 * report the active models, apply the server's assignments and experiment
 * arms. A rejected token triggers re-registration.
 */
internal class HeartbeatManager(
    private val api: DeviceApi,
    private val tokens: TokenStore,
    private val models: ModelManager,
    private val telemetry: TelemetryReporter?,
    private val callback: () -> FleetMLCallback?,
    initialIntervalSeconds: Long,
    /** Called when the server rejects the token (revoked, superseded or expired). */
    private val reRegister: suspend () -> Unit = {},
    /** Called with the DeviceConfig of every heartbeat response that carries one. */
    private val onConfig: (fleetml.device.v1.Device.DeviceConfig) -> Unit = {},
    private val now: () -> Long = System::currentTimeMillis,
) {
    private var heartbeatJob: Job? = null

    @Volatile var intervalSeconds: Long = initialIntervalSeconds.coerceAtLeast(1)
        private set

    @Volatile private var connected: Boolean? = null

    /** Why the last heartbeat did not succeed (null after a success), for the background job's log line. */
    @Volatile internal var lastFailure: Throwable? = null
        private set

    private val inFlightLock = Any()
    private var inFlight: CompletableDeferred<Boolean>? = null

    fun start(scope: CoroutineScope) {
        heartbeatJob?.cancel()
        heartbeatJob = scope.launch {
            while (isActive) {
                beatOnce()
                delay(intervalSeconds * 1000)
            }
        }
    }

    fun stop() {
        heartbeatJob?.cancel()
        heartbeatJob = null
    }

    /** Applies the server's heartbeat interval (DeviceConfig / next_heartbeat_seconds). */
    fun setInterval(seconds: Long) {
        if (seconds > 0) intervalSeconds = seconds
    }

    /**
     * One heartbeat. Returns true if the server was reached. A call while a
     * heartbeat is in flight (the timer's first beat and an app's syncNow() at
     * start-up) waits for that heartbeat's result instead of sending a second
     * one, so its assignments are applied once.
     */
    internal suspend fun beatOnce(): Boolean {
        val (result, mine) = synchronized(inFlightLock) {
            inFlight?.let { it to false } ?: CompletableDeferred<Boolean>().also { inFlight = it }.let { it to true }
        }
        if (!mine) return result.await()
        var reached = false
        try {
            reached = beat()
            return reached
        } finally {
            synchronized(inFlightLock) { inFlight = null }
            result.complete(reached)
        }
    }

    private suspend fun beat(): Boolean {
        val token = tokens.getAuthToken() ?: return false
        val deviceId = tokens.getDeviceId() ?: return false
        return try {
            val current = refreshTokenIfDue(token)
            val response = api.heartbeat(
                current,
                HeartbeatRequest.newBuilder().setDeviceId(deviceId).addAllLoadedModels(models.loadedStatuses()).build(),
            )
            setConnected(true)
            setInterval(response.nextHeartbeatSeconds.toLong())
            if (response.hasConfig()) onConfig(response.config)
            telemetry?.setExperimentArms(response.experimentsList.associate { it.modelVersionId to it.armId })
            // Keys first: an assignment may be signed by a key the device has not seen.
            models.setSigningKeys(response.signingKeysList)
            models.apply(response.assignmentsList)
            lastFailure = null
            true
        } catch (e: StatusException) {
            lastFailure = e
            if (e.status.code == Status.Code.UNAUTHENTICATED) {
                Logger.w("Token rejected by server, re-registering")
                try {
                    reRegister()
                } catch (re: Exception) {
                    lastFailure = re
                    Logger.failure("Re-registration failed", re)
                }
            } else {
                Logger.failure("Heartbeat failed", e)
            }
            setConnected(false)
            false
        } catch (e: Exception) {
            lastFailure = e
            Logger.failure("Heartbeat failed", e)
            setConnected(false)
            false
        }
    }

    /** Swaps the token for a new one shortly before it expires. Returns the token to use. */
    private suspend fun refreshTokenIfDue(token: String): String {
        if (!TokenRefreshPolicy.needsRefresh(now(), tokens.getTokenExpiresAtMillis(), tokens.getTokenIssuedAtMillis())) return token
        return try {
            val response = api.refreshToken(token)
            tokens.saveToken(response.authToken, response.tokenExpiresAt)
            Logger.i("Device token refreshed")
            response.authToken
        } catch (e: StatusException) {
            // FAILED_PRECONDITION (too early, clock skew) is retried on the next heartbeat.
            if (e.status.code == Status.Code.UNAUTHENTICATED) throw e
            Logger.failure("Token refresh failed", e)
            token
        }
    }

    private fun setConnected(value: Boolean) {
        if (connected == value) return
        connected = value
        callback()?.onConnectionStatusChanged(value)
        if (value) telemetry?.onReconnected()
    }
}
