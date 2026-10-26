package com.fleetml.sdk

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotEquals
import org.junit.Test

/** A build with another key, host or port registers again; the same build does not. */
class RegistrationSignatureTest {
    private val labels = mapOf("app" to "sample", "fleet" to "a")
    private fun sig(host: String = "localhost", port: Int = 19090, key: String = "k1", sdk: String = "0.10.2", l: Map<String, String> = labels) =
        DeviceRegistrar.registrationSignature(sdk, l, host, port, key)

    @Test
    fun `the same settings give the same signature, labels in any order`() {
        assertEquals(sig(), sig(l = mapOf("fleet" to "a", "app" to "sample")))
        assertEquals("a key with spaces around is the same key", sig(), sig(key = " k1 "))
    }

    @Test
    fun `a new enrollment key changes the signature`() {
        assertNotEquals(sig(), sig(key = "k2"))
    }

    @Test
    fun `a new host or port changes the signature`() {
        assertNotEquals(sig(), sig(host = "192.0.2.10"))
        assertNotEquals(sig(), sig(port = 29090))
    }

    @Test
    fun `the key itself is not in the signature`() {
        assertFalse(sig(key = "secret-enrollment-key").contains("secret-enrollment-key"))
    }

    @Test
    fun `a signature saved by 0_10_1 differs, so the device registers once more`() {
        val old = "0.10.2|" + labels.toSortedMap().entries.joinToString(",") { "${it.key}=${it.value}" }
        assertNotEquals(old, sig())
    }
}
