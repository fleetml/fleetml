package com.fleetml.sdk

import fleetml.device.v1.Device
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class BatteryPolicyTest {
    private fun may(policy: DownloadPolicy, urgent: Boolean = false, connected: Boolean = true, metered: Boolean = false, charging: Boolean = false) =
        BatteryPolicy.mayDownload(policy, urgent, connected, metered, charging)

    @Test
    fun `the default waits for an unmetered network or a charger`() {
        val p = DownloadPolicy.UNMETERED_OR_CHARGING
        assertTrue("Wi-Fi", may(p))
        assertFalse("mobile data on battery", may(p, metered = true))
        assertTrue("mobile data while charging", may(p, metered = true, charging = true))
    }

    @Test
    fun `unmetered only ignores charging`() {
        assertFalse(may(DownloadPolicy.UNMETERED_ONLY, metered = true, charging = true))
        assertTrue(may(DownloadPolicy.UNMETERED_ONLY))
    }

    @Test
    fun `any network and urgent need only a connection`() {
        assertTrue(may(DownloadPolicy.ANY_NETWORK, metered = true))
        assertTrue(may(DownloadPolicy.UNMETERED_ONLY, urgent = true, metered = true))
        assertFalse("nothing downloads offline", may(DownloadPolicy.ANY_NETWORK, urgent = true, connected = false))
    }

    @Test
    fun `background interval never drops below the OS minimum`() {
        assertEquals(900L, BatteryPolicy.backgroundIntervalSeconds(60))
        assertEquals(3600L, BatteryPolicy.backgroundIntervalSeconds(3600))
        assertEquals("a sync cut short runs again at once", 0L, BatteryPolicy.backgroundDelaySeconds(3600, interrupted = true))
        assertEquals(3600L, BatteryPolicy.backgroundDelaySeconds(3600, interrupted = false))
    }

    @Test
    fun `server policy wins unless unspecified`() {
        assertEquals(DownloadPolicy.ANY_NETWORK, BatteryPolicy.fromProto(Device.DownloadPolicy.ANY_NETWORK, DownloadPolicy.UNMETERED_ONLY))
        assertEquals(DownloadPolicy.UNMETERED_ONLY, BatteryPolicy.fromProto(Device.DownloadPolicy.DOWNLOAD_POLICY_UNSPECIFIED, DownloadPolicy.UNMETERED_ONLY))
    }
}
