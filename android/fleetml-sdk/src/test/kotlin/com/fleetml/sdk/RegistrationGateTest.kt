package com.fleetml.sdk

import com.fleetml.sdk.internal.RegistrationGate
import kotlinx.coroutines.async
import kotlinx.coroutines.awaitAll
import kotlinx.coroutines.delay
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Test

class RegistrationGateTest {
    @Test
    fun `concurrent callers register once`() = runTest {
        var registered = false
        var calls = 0
        val gate = RegistrationGate { !registered }
        (1..3).map {
            async {
                gate.registerIfNeeded {
                    calls++
                    delay(100) // the Register call
                    registered = true
                }
            }
        }.awaitAll()
        assertEquals(1, calls)
    }

    @Test
    fun `a forced registration runs even when registered`() = runTest {
        var calls = 0
        val gate = RegistrationGate { false }
        gate.registerIfNeeded { calls++ }
        gate.register { calls++ }
        assertEquals(1, calls)
    }
}
