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
}
