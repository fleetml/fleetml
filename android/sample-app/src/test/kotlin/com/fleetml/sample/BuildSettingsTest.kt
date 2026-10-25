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
}
