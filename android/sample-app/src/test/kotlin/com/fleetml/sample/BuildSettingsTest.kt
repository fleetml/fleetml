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
    fun emulatorHostByDefault() {
        assertEquals(SampleApp.Settings("10.0.2.2", 19090, "k"), SampleApp.fromBuild(" k ", ""))
    }

    private val build = SampleApp.Settings("localhost", 19090, "new")
    private val saved = SampleApp.Settings("10.0.2.2", 19090, "typed")

    @Test
    fun aNewBuildKeyWinsOverSavedSettings() {
        assertEquals(build to true, SampleApp.startSettings(build, saved, lastBuildKey = "old"))
        assertEquals(build to true, SampleApp.startSettings(build, null, lastBuildKey = null))
    }

    @Test
    fun theSameBuildKeyKeepsWhatTheScreenSaved() {
        assertEquals(saved to false, SampleApp.startSettings(build, saved, lastBuildKey = "new"))
        assertEquals(saved to false, SampleApp.startSettings(null, saved, lastBuildKey = null))
        assertNull(SampleApp.startSettings(null, null, lastBuildKey = null))
    }
}
