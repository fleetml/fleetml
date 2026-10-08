package com.fleetml.sdk

import com.fleetml.sdk.internal.TokenRefreshPolicy
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class TokenRefreshPolicyTest {
    private val day = 24L * 60 * 60 * 1000
    private val now = 1_800_000_000_000L

    @Test
    fun `fresh 30-day token is not refreshed`() {
        assertFalse(TokenRefreshPolicy.needsRefresh(now, now + 30 * day))
    }

    @Test
    fun `token with more than six days left is not refreshed`() {
        assertFalse(TokenRefreshPolicy.needsRefresh(now, now + 6 * day + 1))
    }

    @Test
    fun `token within six days of expiry is refreshed`() {
        assertTrue(TokenRefreshPolicy.needsRefresh(now, now + 6 * day))
        assertTrue(TokenRefreshPolicy.needsRefresh(now, now + 1))
    }

    @Test
    fun `expired token is still flagged so the refresh attempt triggers re-registration`() {
        assertTrue(TokenRefreshPolicy.needsRefresh(now, now - day))
    }

    @Test
    fun `unknown expiry never refreshes`() {
        assertFalse(TokenRefreshPolicy.needsRefresh(now, 0))
    }

    @Test
    fun `sdk window sits inside the server's seven-day window`() {
        assertTrue(TokenRefreshPolicy.REFRESH_BEFORE_EXPIRY_MS < 7 * day)
    }

    @Test
    fun `short-lived tokens refresh at half their lifetime, not every heartbeat`() {
        val issued = now
        val expires = now + day // DEVICE_TOKEN_TTL=24h
        assertFalse(TokenRefreshPolicy.needsRefresh(now + 60_000, expires, issued))
        assertFalse(TokenRefreshPolicy.needsRefresh(now + 11 * 60 * 60 * 1000L, expires, issued))
        assertTrue(TokenRefreshPolicy.needsRefresh(now + 12 * 60 * 60 * 1000L, expires, issued))
    }

    @Test
    fun `long-lived tokens still use the six-day window`() {
        val expires = now + 30 * day
        assertFalse(TokenRefreshPolicy.needsRefresh(now + 23 * day, expires, now))
        assertTrue(TokenRefreshPolicy.needsRefresh(now + 24 * day, expires, now))
    }
}
