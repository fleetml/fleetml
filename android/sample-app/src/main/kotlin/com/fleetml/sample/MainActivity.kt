package com.fleetml.sample

import android.os.Bundle
import android.widget.TextView
import androidx.appcompat.app.AppCompatActivity
import com.fleetml.sdk.FleetML
import com.fleetml.sdk.FleetMLCallback
import com.fleetml.sdk.FleetMLConfig
import java.io.File
import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale

class MainActivity : AppCompatActivity(), FleetMLCallback {
    private lateinit var statusText: TextView
    private lateinit var modelsText: TextView
    private lateinit var logText: TextView

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContentView(R.layout.activity_main)

        statusText = findViewById(R.id.statusText)
        modelsText = findViewById(R.id.modelsText)
        logText = findViewById(R.id.logText)

        appendLog("Initializing FleetML SDK...")

        FleetML.init(
            context = this,
            config = FleetMLConfig(
                serverHost = "10.0.2.2",  // Emulator -> host machine localhost
                serverPort = 19090,
                enrollmentKey = BuildConfig.FLEETML_ENROLLMENT_KEY,
                useTls = false,
                labels = mapOf("env" to "development", "app" to "sample")
            ),
            callback = this
        )
    }

    override fun onModelReady(modelName: String, version: String, variantId: String, modelFile: File) {
        runOnUiThread {
            appendLog("Model ready: $modelName v$version -> ${modelFile.absolutePath}")
            modelsText.append("\n$modelName v$version (${modelFile.length() / 1024}KB)")
        }
    }

    override fun onModelUnload(modelName: String, variantId: String) {
        runOnUiThread { appendLog("Model unloaded: $modelName") }
    }

    override fun onModelDownloadFailed(modelName: String, version: String, reason: String) {
        runOnUiThread { appendLog("Download failed: $modelName v$version - $reason") }
    }

    override fun onRegistered(deviceId: String) {
        runOnUiThread {
            statusText.text = "Registered: $deviceId"
            appendLog("Registered as $deviceId")
        }
    }

    override fun onConnectionStatusChanged(connected: Boolean) {
        runOnUiThread {
            appendLog("Connection: ${if (connected) "OK" else "LOST"}")
        }
    }

    override fun onDestroy() {
        FleetML.stop()
        super.onDestroy()
    }

    private fun appendLog(msg: String) {
        val time = SimpleDateFormat("HH:mm:ss", Locale.US).format(Date())
        logText.append("$time $msg\n")
    }
}
