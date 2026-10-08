package com.fleetml.sdk

import androidx.test.core.app.ApplicationProvider
import android.content.Context
import org.junit.After
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner

@RunWith(RobolectricTestRunner::class)
class FleetMLLifecycleTest {
    private val context: Context = ApplicationProvider.getApplicationContext()

    @After
    fun tearDown() {
        FleetML.stop()
    }

    // An activity that calls init() in onCreate and stop() in onDestroy must
    // work again after a rotation or restart: stop() clears the singleton.
    @Test
    fun `stop clears the singleton so init works again`() {
        val config = FleetMLConfig(serverHost = "127.0.0.1", autoStart = false)
        FleetML.init(context, config)
        assertTrue(FleetML.isInitialized())
        val first = FleetML.getTelemetryReporter()

        FleetML.stop()
        assertFalse(FleetML.isInitialized())

        FleetML.init(context, config)
        assertTrue(FleetML.isInitialized())
        assertTrue("a fresh instance after re-init", first !== FleetML.getTelemetryReporter())
        assertTrue(FleetML.activeModels().isEmpty())
    }

    @Test
    fun `a duplicate init is ignored`() {
        val config = FleetMLConfig(serverHost = "127.0.0.1", autoStart = false)
        FleetML.init(context, config)
        val reporter = FleetML.getTelemetryReporter()
        FleetML.init(context, config)
        assertTrue(reporter === FleetML.getTelemetryReporter())
    }
}
