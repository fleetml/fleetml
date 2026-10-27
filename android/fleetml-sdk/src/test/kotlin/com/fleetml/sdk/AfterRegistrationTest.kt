package com.fleetml.sdk

import org.junit.Assert.assertEquals
import org.junit.Test

class AfterRegistrationTest {
    @Test
    fun `in the foreground the timers run`() {
        assertEquals(AfterRegistration.TIMERS, afterRegistration(foreground = true, processStarted = true))
    }

    // A foreground cold start with a stored registration: registration is done
    // before the lifecycle observer has reported the start. The process is
    // started, so onForeground starts the timers; no background job (0.10.4
    // scheduled one and logged "In the background" at every cold start).
    @Test
    fun `registered before the observer saw a started process, nothing is scheduled`() {
        assertEquals(AfterRegistration.NOTHING, afterRegistration(foreground = false, processStarted = true))
    }

    // Home pressed before the first screen: the process never started, no
    // onStop will come, so the background job must take the first sync.
    @Test
    fun `registered while the process never started, the background job finishes the first sync`() {
        assertEquals(AfterRegistration.BACKGROUND_JOB, afterRegistration(foreground = false, processStarted = false))
    }
}
