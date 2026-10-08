package com.fleetml.sdk

import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import kotlinx.coroutines.runBlocking
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Assume.assumeTrue
import org.junit.Test
import org.junit.runner.RunWith

/**
 * One phase of scripts/signing-e2e.sh: the models in expectActive must become
 * active and those in expectRejected must not (their signature does not
 * verify). Skipped unless the script passes the arguments.
 */
@RunWith(AndroidJUnit4::class)
class SignatureLiveTest {
    private val args = InstrumentationRegistry.getArguments()
    private val context = InstrumentationRegistry.getInstrumentation().targetContext

    @After
    fun tearDown() {
        FleetML.stop()
    }

    @Test
    fun signaturesAreVerifiedBeforeActivation() {
        val enrollmentKey = args.getString("enrollmentKey")
        assumeTrue("run via scripts/signing-e2e.sh android", !enrollmentKey.isNullOrEmpty())
        val expectActive = args.getString("expectActive")!!.split(",").toSet()
        val expectRejected = args.getString("expectRejected")!!.split(",").toSet()
        val rejected = mutableListOf<String>()

        // The same device across the script's phases: it keeps the keys it was sent.
        FleetML.stop()
        FleetML.init(
            context,
            FleetMLConfig(serverHost = args.getString("host", "10.0.2.2"), serverPort = args.getString("grpcPort", "19090").toInt(),
                enrollmentKey = enrollmentKey!!, labels = mapOf("e2e-run" to args.getString("label")!!)),
            object : FleetMLCallback {
                override fun onModelLoadFailed(modelName: String, version: String, reason: String) {
                    synchronized(rejected) { rejected += "$modelName: $reason" }
                }
            },
        )
        fun active() = FleetML.activeModels().map { it.modelName }.toSet()
        var waited = 0
        while (!active().containsAll(expectActive) && waited < 90) {
            runBlocking { FleetML.syncNow() }
            Thread.sleep(2000)
            waited += 2
        }
        runBlocking { FleetML.syncNow() }
        assertTrue("active ${active()} contains $expectActive", active().containsAll(expectActive))
        assertTrue("none of $expectRejected is active: ${active()}", active().none { it in expectRejected })
        for (name in expectRejected) {
            assertTrue("$name was rejected for its signature: $rejected",
                synchronized(rejected) { rejected.any { it.startsWith("$name: signature:") } })
        }
        assertEquals(true, runBlocking { FleetML.getTelemetryReporter().flushNow() })
        android.util.Log.i("FleetMLSignatureTest", "active=${active()} rejected=$rejected")
    }
}
