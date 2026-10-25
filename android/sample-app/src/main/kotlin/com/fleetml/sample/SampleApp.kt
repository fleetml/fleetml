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
    /** The assigned model that last failed to download or validate, with the reason; null once a model is ready. */
    @Volatile var failing: String? = null

    override fun onCreate() {
        super.onCreate()
        // A build that carries an enrollment key (-PfleetmlEnrollmentKey or
        // ORG_GRADLE_PROJECT_fleetmlEnrollmentKey) connects to the build's host
        // on its first start, without a tap on Save and connect, and again
        // whenever a new build brings another key; otherwise the saved settings
        // (the config screen's) win.
        val prefs = getSharedPreferences(PREFS, Context.MODE_PRIVATE)
        val build = fromBuild(BuildConfig.FLEETML_ENROLLMENT_KEY, BuildConfig.FLEETML_DEBUG_HOST)
        val (s, fromBuild) = startSettings(build, settings(), prefs.getString(BUILD_KEY, null)) ?: return
        if (fromBuild) {
            prefs.edit().putString(BUILD_KEY, s.enrollmentKey).apply()
            connect(s)
        } else {
            startSdk(s)
        }
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
            labels = mapOf("app" to "sample")), callback = this)
    }

    fun observe(listener: () -> Unit) = listeners.add(listener)
    fun unobserve(listener: () -> Unit) = listeners.remove(listener)

    fun note(message: String) {
        log.add(0, "${SimpleDateFormat("HH:mm:ss", Locale.US).format(Date())}  $message")
        while (log.size > 200) log.removeAt(log.size - 1)
        listeners.forEach { it() }
    }

    override fun onModelReady(model: ActiveModel) {
        failing = null
        note("Model ready: ${model.modelName} ${model.version} (${model.file.length()} bytes)")
    }
    override fun onModelUnload(modelId: String, modelName: String, variantId: String) = note("Model unloaded: $modelName")
    override fun onModelLoadFailed(modelName: String, version: String, reason: String) {
        failing = "$modelName $version: $reason"
        note("Model failed: $modelName $version: $reason")
    }
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
        /** The build key last applied, so a reinstall of the same build keeps what the config screen saved. */
        private const val BUILD_KEY = "build_enrollment_key"

        /** The settings a build carries: its key and debug host (the emulator's 10.0.2.2 when none), or null without a key. */
        fun fromBuild(enrollmentKey: String, debugHost: String): Settings? =
            if (enrollmentKey.isBlank()) null else Settings(debugHost.ifBlank { "10.0.2.2" }, 19090, enrollmentKey.trim())

        /** What to start with, and whether it is the build's: a build key not applied before, else the saved settings. */
        fun startSettings(build: Settings?, saved: Settings?, lastBuildKey: String?): Pair<Settings, Boolean>? = when {
            build != null && build.enrollmentKey != lastBuildKey -> build to true
            saved != null -> saved to false
            else -> null
        }
    }
}
