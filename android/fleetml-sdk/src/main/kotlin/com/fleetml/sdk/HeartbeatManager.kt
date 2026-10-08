package com.fleetml.sdk

import com.fleetml.sdk.internal.GrpcClient
import com.fleetml.sdk.internal.Logger
import com.fleetml.sdk.internal.TokenManager
import com.fleetml.sdk.internal.TokenRefreshPolicy
import fleetml.device.v1.Device.HeartbeatRequest
import fleetml.device.v1.Device.ModelAssignment
import fleetml.device.v1.Device.ModelStatus
import fleetml.device.v1.Device.RefreshTokenRequest
import io.grpc.Status
import io.grpc.StatusException
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch
import java.util.concurrent.ConcurrentHashMap

/**
 * Manages the periodic heartbeat loop with the FleetML server.
 *
 * Sends loaded model status, receives assignments, and triggers downloads/unloads.
 */
internal class HeartbeatManager(
    private val grpcClient: GrpcClient,
    private val tokenManager: TokenManager,
    private val downloader: ModelDownloader,
    private val cache: ModelCache,
    private val callback: FleetMLCallback?,
    private val config: FleetMLConfig,
    /** Called when the server rejects the token (revoked, superseded or expired). */
    private val reRegister: suspend () -> Unit = {}
) {
    private var heartbeatJob: Job? = null
    private var intervalSeconds: Long = config.heartbeatIntervalSeconds
    private val loadedModels = ConcurrentHashMap<String, LoadedModelInfo>()

    /** Start the periodic heartbeat loop. */
    fun start(scope: CoroutineScope) {
        heartbeatJob?.cancel()
        heartbeatJob = scope.launch {
            while (isActive) {
                try {
                    refreshTokenIfDue()
                    sendHeartbeat()
                } catch (e: StatusException) {
                    if (e.status.code == Status.Code.UNAUTHENTICATED) {
                        Logger.w("Token rejected by server, re-registering")
                        try {
                            reRegister()
                        } catch (re: Exception) {
                            Logger.w("Re-registration failed: ${re.message}")
                        }
                    } else {
                        Logger.w("Heartbeat failed: ${e.message}")
                    }
                    callback?.onConnectionStatusChanged(false)
                } catch (e: Exception) {
                    Logger.w("Heartbeat failed: ${e.message}")
                    callback?.onConnectionStatusChanged(false)
                }
                delay(intervalSeconds * 1000)
            }
        }
    }

    /** Stop the heartbeat loop. */
    fun stop() {
        heartbeatJob?.cancel()
        heartbeatJob = null
    }

    /** Swap the token for a new one shortly before it expires. */
    private suspend fun refreshTokenIfDue() {
        val token = tokenManager.getAuthToken() ?: return
        if (!TokenRefreshPolicy.needsRefresh(System.currentTimeMillis(), tokenManager.getTokenExpiresAtMillis())) return
        try {
            val response = grpcClient.authenticatedStub(token).refreshToken(RefreshTokenRequest.getDefaultInstance())
            tokenManager.saveToken(response.authToken, response.tokenExpiresAt)
            Logger.i("Device token refreshed")
        } catch (e: StatusException) {
            // FAILED_PRECONDITION (too early, clock skew) is retried on the next heartbeat.
            if (e.status.code == Status.Code.UNAUTHENTICATED) throw e
            Logger.w("Token refresh failed: ${e.status}")
        }
    }

    private suspend fun sendHeartbeat() {
        val token = tokenManager.getAuthToken() ?: return
        val deviceId = tokenManager.getDeviceId() ?: return

        val stub = grpcClient.authenticatedStub(token)

        val request = HeartbeatRequest.newBuilder()
            .setDeviceId(deviceId)
            .addAllLoadedModels(
                loadedModels.values.map { model ->
                    ModelStatus.newBuilder()
                        .setModelId(model.modelId)
                        .setVersion(model.version)
                        .setVariantId(model.variantId)
                        .setStatus("LOADED")
                        .build()
                }
            )
            .build()

        val response = stub.heartbeat(request)
        callback?.onConnectionStatusChanged(true)

        if (response.nextHeartbeatSeconds > 0) {
            intervalSeconds = response.nextHeartbeatSeconds.toLong()
        }

        for (assignment in response.assignmentsList) {
            processAssignment(assignment)
        }
    }

    private suspend fun processAssignment(assignment: ModelAssignment) {
        when (assignment.action) {
            ModelAssignment.Action.LOAD,
            ModelAssignment.Action.UPDATE -> {
                val result = downloader.download(
                    variantId = assignment.variantId,
                    downloadUrl = assignment.downloadUrl,
                    blobSha256 = assignment.blobSha256
                )
                when (result) {
                    is DownloadResult.Downloaded,
                    is DownloadResult.Cached -> {
                        val file = when (result) {
                            is DownloadResult.Downloaded -> result.file
                            is DownloadResult.Cached -> result.file
                            else -> return
                        }
                        loadedModels[assignment.variantId] = LoadedModelInfo(
                            modelId = assignment.modelId,
                            modelName = assignment.modelName,
                            version = assignment.version,
                            variantId = assignment.variantId
                        )
                        callback?.onModelReady(
                            assignment.modelName,
                            assignment.version,
                            assignment.variantId,
                            file
                        )
                    }
                    is DownloadResult.Failed -> {
                        callback?.onModelDownloadFailed(
                            assignment.modelName,
                            assignment.version,
                            result.reason
                        )
                    }
                }
            }
            ModelAssignment.Action.UNLOAD -> {
                loadedModels.remove(assignment.variantId)
                callback?.onModelUnload(assignment.modelName, assignment.variantId)
            }
            else -> { /* ACTION_UNSPECIFIED — ignore */ }
        }
    }
}

internal data class LoadedModelInfo(
    val modelId: String,
    val modelName: String,
    val version: String,
    val variantId: String
)
