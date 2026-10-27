package com.fleetml.sdk

import org.junit.Assert.assertEquals
import org.junit.Test

class AfterRegistrationTest {
    @Test
    fun `in the foreground the timers run`() {
        assertEquals(AfterRegistration.TIMERS, afterRegistration(foreground = true))
    }

    // Home pressed before the first screen: no onStop ever comes, so the
    // background job must take the first sync, or nothing syncs until reopened.
    @Test
    fun `registered in the background, the background job finishes the first sync`() {
        assertEquals(AfterRegistration.BACKGROUND_JOB, afterRegistration(foreground = false))
    }
}
