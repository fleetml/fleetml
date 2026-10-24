package com.fleetml.sample

import android.os.Bundle
import android.widget.Button
import android.widget.EditText
import android.widget.TextView
import androidx.appcompat.app.AppCompatActivity
import com.fleetml.sdk.FleetML
import java.util.concurrent.Executors

class MainActivity : AppCompatActivity() {
    private val app get() = application as SampleApp
    private val worker = Executors.newSingleThreadExecutor()
    private lateinit var status: TextView
    private lateinit var model: TextView
    private lateinit var result: TextView
    private lateinit var logView: TextView
    private val refresh: () -> Unit = { runOnUiThread { render() } }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContentView(R.layout.activity_main)
        status = findViewById(R.id.statusText)
        model = findViewById(R.id.modelText)
        result = findViewById(R.id.resultText)
        logView = findViewById(R.id.logText)
        val host = findViewById<EditText>(R.id.hostInput)
        val port = findViewById<EditText>(R.id.portInput)
        val key = findViewById<EditText>(R.id.keyInput)

        val saved = app.settings()
        host.setText(saved?.host ?: "10.0.2.2")
        port.setText((saved?.port ?: 19090).toString())
        key.setText(saved?.enrollmentKey ?: BuildConfig.FLEETML_ENROLLMENT_KEY)

        findViewById<Button>(R.id.connectButton).setOnClickListener {
            app.connect(SampleApp.Settings(host.text.toString().trim(), port.text.toString().toIntOrNull() ?: 19090, key.text.toString().trim()))
        }
        findViewById<Button>(R.id.runButton).setOnClickListener { worker.execute { runInference() } }
    }

    override fun onStart() {
        super.onStart()
        app.observe(refresh)
        render()
    }

    override fun onStop() {
        app.unobserve(refresh)
        super.onStop()
    }

    /** Runs the active FleetML model (or the bundled one) on the sample input and reports telemetry. */
    private fun runInference() {
        val active = if (FleetML.isInitialized()) FleetML.activeModels().firstOrNull() else null
        val file = active?.file ?: IrisClassifier.bundledModel(this)
        val text = try {
            val r = IrisClassifier(file).classify(IrisClassifier.SAMPLE_INPUT)
            if (active != null) {
                FleetML.getTelemetryReporter().reportInference(active, r.latencyMs, r.confidence, r.label)
            }
            val source = active?.let { "${it.modelName} ${it.version}" } ?: "bundled model"
            app.note("Inference with $source: ${r.label} (${"%.2f".format(r.confidence)}) in ${r.latencyMicros} µs")
            "${r.label}  ${"%.1f".format(r.confidence * 100)}%  ${r.latencyMicros} µs\n($source)"
        } catch (e: Exception) {
            if (active != null) {
                FleetML.getTelemetryReporter().reportInference(active, 0, errorMessage = e.message ?: "inference failed")
            }
            app.note("Inference failed: ${e.message}")
            "error: ${e.message}"
        }
        runOnUiThread { result.text = text }
    }

    private fun render() {
        // Online means the server answers; the assigned model can still fail to download (a CLEARTEXT block, say) or validate.
        val connected = when (app.connected) { true -> if (app.failing != null) "online, assigned model failing" else "online"; false -> "offline"; null -> "connecting" }
        val id = FleetML.deviceId() ?: app.deviceId
        status.text = if (!FleetML.isInitialized()) "Not configured" else "Device ${id ?: "(registering)"} · $connected"
        val active = if (FleetML.isInitialized()) FleetML.activeModels() else emptyList()
        val failing = app.failing
        model.text = if (active.isEmpty() && failing != null) "Active model: bundled iris (assigned $failing)"
        else if (active.isEmpty()) "Active model: bundled iris (no assignment yet)"
        else "Active model: " + active.joinToString { "${it.modelName} ${it.version}" }
        logView.text = app.log.joinToString("\n")
    }
}
