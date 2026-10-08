package com.fleetml.sdk

import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import kotlinx.coroutines.runBlocking
import org.json.JSONObject
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Assume.assumeTrue
import org.junit.Test
import org.junit.runner.RunWith
import org.tensorflow.lite.Interpreter
import java.net.HttpURLConnection
import java.net.URL
import java.security.MessageDigest
import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit

/**
 * Register -> download -> validate/activate -> inference telemetry against a
 * running FleetML dev stack. Run it with scripts/android-e2e.sh, which creates
 * the project, version, enrollment key and deployment and passes them in as
 * instrumentation arguments. Skipped when the arguments are missing.
 */
@RunWith(AndroidJUnit4::class)
class LiveServerTest {
    private val args = InstrumentationRegistry.getArguments()
    private val context = InstrumentationRegistry.getInstrumentation().targetContext

    @After
    fun tearDown() {
        FleetML.reset()
        DeviceFingerprint.override = null
    }

    @Test
    fun registerDownloadActivateAndReport() {
        val enrollmentKey = args.getString("enrollmentKey")
        assumeTrue("run via scripts/android-e2e.sh", !enrollmentKey.isNullOrEmpty())
        val host = args.getString("host", "10.0.2.2")
        val expectedVersion = args.getString("expectedVersion")!!
        val expectedSha = args.getString("expectedSha")!!

        // Each run uses a new project, and the previous run's tearDown cleared
        // the stored token: register as a device of this run's own.
        FleetML.stop()
        DeviceFingerprint.override = sha256(args.getString("label")!!.toByteArray())
        val ready = CountDownLatch(1)
        var active: ActiveModel? = null
        FleetML.init(
            context,
            FleetMLConfig(serverHost = host, serverPort = args.getString("grpcPort", "19090").toInt(),
                enrollmentKey = enrollmentKey!!, labels = mapOf("e2e-run" to args.getString("label")!!)),
            object : FleetMLCallback {
                override fun onModelReady(model: ActiveModel) {
                    // Models another live test left in the app's storage are reported first (offline start).
                    if (model.modelName != "iris" || model.version != expectedVersion) return
                    active = model
                    ready.countDown()
                }
            },
        )

        // The test process has no activity, so the SDK is "in the background" and
        // its timers do not run: drive it the way the background job does.
        var waited = 0
        while (!ready.await(2, TimeUnit.SECONDS) && waited < 90) {
            runBlocking { FleetML.syncNow() }
            waited += 2
        }
        assertTrue("model assigned, downloaded and validated within 90s", ready.count == 0L)
        val model = active!!
        assertEquals(expectedVersion, model.version)
        assertEquals(expectedSha, sha256(model.file.readBytes()))
        assertTrue(FleetML.isRegistered())

        // Run the model the SDK activated and report the inference.
        val output = arrayOf(FloatArray(3))
        Interpreter(model.file).use { it.run(arrayOf(floatArrayOf(6.7f, 3.0f, 5.2f, 2.3f)), output) }
        assertEquals("virginica is class 2", 2, output[0].indices.maxByOrNull { output[0][it] })
        val reporter = FleetML.getTelemetryReporter()
        reporter.reportInference(model, latencyMs = 1, confidence = output[0][2], predictionLabel = "virginica")
        assertTrue("telemetry delivered", runBlocking { reporter.flushNow() })

        // The server stored it (the consumer flushes every 5s).
        val deviceId = FleetML.deviceId()!!
        val api = "http://$host:${args.getString("restPort", "18080")}"
        var stored = 0
        repeat(20) {
            if (stored == 0) {
                Thread.sleep(1000)
                stored = countInferenceEvents(api, args.getString("apiKey")!!, deviceId, model.versionId)
            }
        }
        assertTrue("inference event stored server-side", stored >= 1)
    }

    private fun countInferenceEvents(api: String, key: String, deviceId: String, versionId: String): Int {
        val url = URL("$api/api/v1/telemetry/events?device_id=$deviceId&model_version_id=$versionId&event_type=INFERENCE&from=2020-01-01T00:00:00Z")
        val conn = url.openConnection() as HttpURLConnection
        conn.setRequestProperty("Authorization", "Bearer $key")
        return conn.inputStream.use { JSONObject(it.reader().readText()).getJSONObject("pagination").getInt("total") }
    }

    private fun sha256(b: ByteArray) = MessageDigest.getInstance("SHA-256").digest(b).joinToString("") { "%02x".format(it) }
}
