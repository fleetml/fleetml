package com.fleetml.sdk

import com.fleetml.sdk.internal.GrpcClient
import com.fleetml.sdk.internal.Logger
import com.fleetml.sdk.internal.TokenManager
import fleetml.device.v1.Device.HeartbeatRequest
import fleetml.device.v1.Device.ModelAssignment
import fleetml.device.v1.Device.ModelStatus
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
    private val config: FleetMLConfig
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
                    sendHeartbeat()
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
