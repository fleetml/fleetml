package com.fleetml.sdk.internal

import com.fleetml.sdk.ActiveModel
import com.fleetml.sdk.DownloadResult
import com.fleetml.sdk.FleetMLCallback
import com.fleetml.sdk.ModelCache
import com.fleetml.sdk.ModelDownloader
import com.fleetml.sdk.ModelValidator
import com.fleetml.sdk.TelemetryReporter
import com.fleetml.sdk.ValidationResult
import fleetml.device.v1.Device.ModelAssignment
import fleetml.device.v1.Device.ModelStatus
import fleetml.telemetry.v1.Telemetry.TelemetryEvent

/**
 * Applies the server's assignments: download (resumable, sha256-verified),
 * validate, and only then activate. A model that fails validation never
 * replaces the active one; the last-known-good file is never deleted.
 */
internal class ModelManager(
    private val store: ModelStore,
    private val cache: ModelCache,
    private val downloader: ModelDownloader,
    private val validator: ModelValidator,
    private val telemetry: TelemetryReporter?,
    private val callback: () -> FleetMLCallback?,
    private val now: () -> Long = System::currentTimeMillis,
    /** Whether a download may start now; the argument is the assignment's urgent flag. */
    private val canDownload: (urgent: Boolean) -> Boolean = { true },
) {
    /** What the device reports in its heartbeat: the active (validated) models only. */
    fun loadedStatuses(): List<ModelStatus> = store.activeModels().map {
        ModelStatus.newBuilder().setModelId(it.modelId).setVersion(it.version).setVariantId(it.variantId).setStatus("LOADED").build()
    }

    /** Active models whose files are on disk (used for offline start). */
    fun activeModels(): List<ActiveModel> = store.activeModels().mapNotNull { rec ->
        cache.getModelFile(rec.variantId)?.let { rec.toActive(it) }
    }

    suspend fun apply(assignments: List<ModelAssignment>) {
        // A model being replaced arrives as UPDATE(new) + UNLOAD(old). The old
        // version must stay until the new one is validated, so such UNLOADs are
        // ignored: activation itself retires the old version.
        val replacing = assignments
            .filter { it.action == ModelAssignment.Action.LOAD || it.action == ModelAssignment.Action.UPDATE }
            .map { it.modelId }.toSet()
        for (a in assignments) {
            when (a.action) {
                ModelAssignment.Action.LOAD, ModelAssignment.Action.UPDATE -> install(a)
                ModelAssignment.Action.UNLOAD -> unload(a, replacing)
                else -> Unit
            }
        }
    }

    private suspend fun install(a: ModelAssignment) {
        if (store.isActive(a.variantId)) return
        if (store.recentlyFailed(a.variantId, now())) return
        if (cache.getModelFile(a.variantId) == null && !canDownload(a.urgent)) {
            Logger.i("Download of ${a.modelName} ${a.version} waits for the download policy (unmetered network or charging)")
            return // the assignment is repeated on the next heartbeat
        }

        val file = when (val r = downloader.download(a.variantId, a.downloadUrl, a.blobSha256, a.blobSizeBytes, store.protectedVariants())) {
            is DownloadResult.Downloaded -> r.file
            is DownloadResult.Cached -> r.file
            is DownloadResult.Failed -> {
                callback()?.onModelLoadFailed(a.modelName, a.version, r.reason)
                return // retried on the next heartbeat
            }
        }

        val record = ModelRecord(a.modelId, a.modelName, a.version, a.versionId, a.variantId, a.blobSha256)
        val candidate = record.toActive(file)
        val started = now()
        val verdict = try {
            validator.validate(file, candidate)
        } catch (e: Throwable) {
            ValidationResult.Invalid(e.message ?: e.javaClass.simpleName)
        }
        when (verdict) {
            is ValidationResult.Invalid -> {
                Logger.w("Model ${a.modelName} ${a.version} failed validation: ${verdict.reason}; keeping the current model")
                store.markFailed(a.variantId, now())
                if (a.variantId !in store.protectedVariants()) cache.delete(a.variantId)
                telemetry?.reportModelStatus(a.versionId, TelemetryEvent.EventType.MODEL_LOAD_FAILED, errorMessage = verdict.reason)
                callback()?.onModelLoadFailed(a.modelName, a.version, verdict.reason)
            }
            is ValidationResult.Valid -> {
                store.activate(record)
                telemetry?.reportModelStatus(a.versionId, TelemetryEvent.EventType.MODEL_LOAD, latencyMs = (now() - started).toInt())
                Logger.i("Model ${a.modelName} ${a.version} is active")
                callback()?.onModelReady(candidate)
            }
        }
    }

    private fun unload(a: ModelAssignment, replacing: Set<String>) {
        val active = store.activeModels().firstOrNull { it.variantId == a.variantId } ?: return
        if (active.modelId in replacing) return
        store.unload(active.modelId)
        callback()?.onModelUnload(active.modelId, active.modelName, active.variantId)
    }

    private fun ModelRecord.toActive(file: java.io.File) =
        ActiveModel(modelId, modelName, version, versionId, variantId, sha256, file)
}
