package com.fleetml.sample

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

/** A build with an enrollment key connects on its first start; one without waits for the config screen. */
class BuildSettingsTest {
    @Test
    fun noKeyMeansNoSettings() {
        assertNull(SampleApp.fromBuild("", "localhost"))
        assertNull(SampleApp.fromBuild("  ", ""))
    }

    @Test
    fun keyAndHostFromTheBuild() {
        assertEquals(SampleApp.Settings("localhost", 19090, "k"), SampleApp.fromBuild("k", "localhost"))
    }

    @Test
    fun portFromTheBuild() {
        assertEquals(SampleApp.Settings("localhost", 29090, "k"), SampleApp.fromBuild("k", "localhost", 29090))
    }

    @Test
    fun emulatorHostByDefault() {
        assertEquals(SampleApp.Settings("10.0.2.2", 19090, "k"), SampleApp.fromBuild(" k ", ""))
    }

    private val build = SampleApp.Settings("localhost", 19090, "new")
    private val saved = SampleApp.Settings("10.0.2.2", 19090, "typed")

    @Test
    fun aNewBuildKeyWinsOverSavedSettings() {
        assertEquals(build to true, SampleApp.startSettings(build, saved, lastBuild = "localhost:19090:old"))
        assertEquals(build to true, SampleApp.startSettings(build, null, lastBuild = null))
    }

    @Test
    fun aNewBuildHostOrPortWinsWithTheSameKey() {
        // The Wi-Fi case: the same key, rebuilt with the machine's LAN address.
        val wifi = SampleApp.Settings("192.0.2.10", 19090, "new")
        assertEquals(wifi to true, SampleApp.startSettings(wifi, saved, lastBuild = "localhost:19090:new"))
        val moved = SampleApp.Settings("localhost", 29090, "new")
        assertEquals(moved to true, SampleApp.startSettings(moved, saved, lastBuild = "localhost:19090:new"))
    }

    @Test
    fun theSameBuildKeepsWhatTheScreenSaved() {
        assertEquals(saved to false, SampleApp.startSettings(build, saved, lastBuild = "localhost:19090:new"))
        assertEquals(saved to false, SampleApp.startSettings(null, saved, lastBuild = null))
        assertNull(SampleApp.startSettings(null, null, lastBuild = null))
    }

    @Test
    fun aBuildFrom0100IsAppliedOnce() {
        // 0.10.0 saved only the key under another name, so the first start of this build applies it again.
        assertEquals(build to true, SampleApp.startSettings(build, saved, lastBuild = null))
    }
}
