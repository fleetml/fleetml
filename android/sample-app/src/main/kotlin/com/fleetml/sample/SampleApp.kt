package com.fleetml.sample

import android.app.Application
import android.content.Context
import com.fleetml.sdk.ActiveModel
import com.fleetml.sdk.FleetML
import com.fleetml.sdk.FleetMLCallback
import com.fleetml.sdk.FleetMLConfig
import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale
import java.util.concurrent.CopyOnWriteArrayList

/**
 * Owns the SDK for the whole process, so screen rotation or a recreated
 * activity never re-initializes it. The activity observes [events].
 */
class SampleApp : Application(), FleetMLCallback {
    data class Settings(val host: String, val port: Int, val enrollmentKey: String)

    val log = CopyOnWriteArrayList<String>()
    private val listeners = CopyOnWriteArrayList<() -> Unit>()
    @Volatile var deviceId: String? = null
    @Volatile var connected: Boolean? = null

    override fun onCreate() {
        super.onCreate()
        settings()?.let { startSdk(it) }
    }

    fun settings(): Settings? {
        val p = getSharedPreferences(PREFS, Context.MODE_PRIVATE)
        val host = p.getString("host", null) ?: return null
        return Settings(host, p.getInt("port", 19090), p.getString("enrollment_key", "").orEmpty())
    }

    /** Saves the settings and (re)starts the SDK with them. */
    fun connect(s: Settings) {
        getSharedPreferences(PREFS, Context.MODE_PRIVATE).edit()
            .putString("host", s.host).putInt("port", s.port).putString("enrollment_key", s.enrollmentKey).apply()
        FleetML.stop()
        startSdk(s)
    }

    private fun startSdk(s: Settings) {
        note("Starting FleetML against ${s.host}:${s.port}")
        FleetML.init(this, FleetMLConfig(serverHost = s.host, serverPort = s.port, enrollmentKey = s.enrollmentKey,
            labels = mapOf("app" to "sample"), downloadOnWifiOnly = false), callback = this)
    }

    fun observe(listener: () -> Unit) = listeners.add(listener)
    fun unobserve(listener: () -> Unit) = listeners.remove(listener)

    fun note(message: String) {
        log.add(0, "${SimpleDateFormat("HH:mm:ss", Locale.US).format(Date())}  $message")
        while (log.size > 200) log.removeAt(log.size - 1)
        listeners.forEach { it() }
    }

    override fun onModelReady(model: ActiveModel) = note("Model ready: ${model.modelName} ${model.version} (${model.file.length()} bytes)")
    override fun onModelUnload(modelId: String, modelName: String, variantId: String) = note("Model unloaded: $modelName")
    override fun onModelLoadFailed(modelName: String, version: String, reason: String) = note("Model failed: $modelName $version: $reason")
    override fun onRegistered(deviceId: String) {
        this.deviceId = deviceId
        note("Registered as $deviceId")
    }
    override fun onConnectionStatusChanged(connected: Boolean) {
        this.connected = connected
        note(if (connected) "Server reachable" else "Server unreachable (offline)")
    }

    companion object {
        private const val PREFS = "sample_settings"
    }
}
