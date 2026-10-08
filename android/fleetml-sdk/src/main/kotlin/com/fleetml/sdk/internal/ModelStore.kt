package com.fleetml.sdk.internal

import org.json.JSONObject
import java.io.File

/** A model version the device has (or had) running. */
internal data class ModelRecord(
    val modelId: String,
    val modelName: String,
    val version: String,
    val versionId: String,
    val variantId: String,
    val sha256: String,
) {
    fun toJson(): JSONObject = JSONObject()
        .put("model_id", modelId).put("model_name", modelName).put("version", version)
        .put("version_id", versionId).put("variant_id", variantId).put("sha256", sha256)

    companion object {
        fun fromJson(o: JSONObject) = ModelRecord(
            o.getString("model_id"), o.optString("model_name"), o.optString("version"),
            o.optString("version_id"), o.getString("variant_id"), o.optString("sha256"),
        )
    }
}

/**
 * Persists, per model, the active version and the last-known-good one (the
 * version that was active before it), plus variants that failed validation.
 * Survives process death so the app can start with its cached model offline.
 */
internal class ModelStore(private val file: File) {
    private val lock = Any()
    private val active = LinkedHashMap<String, ModelRecord>()
    private val lastKnownGood = HashMap<String, ModelRecord>()
    private val failed = HashMap<String, Long>() // variant ID -> failed at (ms)

    init {
        file.parentFile?.mkdirs()
        load()
    }

    fun activeModels(): List<ModelRecord> = synchronized(lock) { active.values.toList() }

    fun active(modelId: String): ModelRecord? = synchronized(lock) { active[modelId] }

    fun lastKnownGood(modelId: String): ModelRecord? = synchronized(lock) { lastKnownGood[modelId] }

    fun isActive(variantId: String): Boolean = synchronized(lock) { active.values.any { it.variantId == variantId } }

    /** Variants that eviction must keep: every active and last-known-good model. */
    fun protectedVariants(): Set<String> = synchronized(lock) {
        (active.values.map { it.variantId } + lastKnownGood.values.map { it.variantId }).toSet()
    }

    /** Makes [record] active; the model's previous active version becomes last-known-good. */
    fun activate(record: ModelRecord) = synchronized(lock) {
        val previous = active[record.modelId]
        if (previous != null && previous.variantId != record.variantId) lastKnownGood[record.modelId] = previous
        active[record.modelId] = record
        failed.remove(record.variantId)
        persist()
    }

    /** The server no longer assigns this model: forget it. */
    fun unload(modelId: String) = synchronized(lock) {
        active.remove(modelId)
        lastKnownGood.remove(modelId)
        persist()
    }

    fun markFailed(variantId: String, at: Long) = synchronized(lock) {
        failed[variantId] = at
        persist()
    }

    /** A variant that failed validation is not retried until [cooldownMs] has passed. */
    fun recentlyFailed(variantId: String, now: Long, cooldownMs: Long = FAILED_COOLDOWN_MS): Boolean =
        synchronized(lock) { failed[variantId]?.let { now - it < cooldownMs } ?: false }

    private fun persist() {
        val json = JSONObject()
        val models = JSONObject()
        for ((id, rec) in active) {
            models.put(id, JSONObject().put("active", rec.toJson()).also { o -> lastKnownGood[id]?.let { o.put("last_known_good", it.toJson()) } })
        }
        for ((id, rec) in lastKnownGood) if (!active.containsKey(id)) models.put(id, JSONObject().put("last_known_good", rec.toJson()))
        json.put("models", models)
        json.put("failed", JSONObject(failed.mapValues { it.value }))
        val tmp = File(file.parentFile, file.name + ".tmp")
        tmp.writeText(json.toString())
        if (!tmp.renameTo(file)) {
            file.delete()
            tmp.renameTo(file)
        }
    }

    private fun load() {
        if (!file.exists()) return
        try {
            val json = JSONObject(file.readText())
            val models = json.optJSONObject("models") ?: JSONObject()
            for (id in models.keys()) {
                val m = models.getJSONObject(id)
                m.optJSONObject("active")?.let { active[id] = ModelRecord.fromJson(it) }
                m.optJSONObject("last_known_good")?.let { lastKnownGood[id] = ModelRecord.fromJson(it) }
            }
            val f = json.optJSONObject("failed") ?: JSONObject()
            for (v in f.keys()) failed[v] = f.getLong(v)
        } catch (e: Exception) {
            Logger.w("Model store unreadable, starting empty: ${e.message}")
        }
    }

    companion object {
        const val FAILED_COOLDOWN_MS = 60L * 60 * 1000
    }
}
