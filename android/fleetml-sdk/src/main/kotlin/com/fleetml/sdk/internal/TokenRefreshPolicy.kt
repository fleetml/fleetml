package com.fleetml.sdk.internal

/**
 * Decides when to refresh the device token.
 *
 * The server accepts RefreshToken in the last 7 days of a token's life. The SDK
 * asks one day later than that (6 days before expiry) so a device clock that runs
 * up to a day fast still lands inside the server's window. For short-lived
 * tokens (DEVICE_TOKEN_TTL under 12 days) it waits until half the lifetime has
 * passed, so a 24h token is refreshed every 12h, not on every heartbeat.
 */
internal object TokenRefreshPolicy {
    const val REFRESH_BEFORE_EXPIRY_MS: Long = 6L * 24 * 60 * 60 * 1000

    /**
     * True when a known expiry is close enough to refresh. Unknown expiry (0)
     * never refreshes. [issuedAtMs] (0 if unknown) shortens the window to half
     * the token's lifetime.
     */
    fun needsRefresh(nowMs: Long, expiresAtMs: Long, issuedAtMs: Long = 0, windowMs: Long = REFRESH_BEFORE_EXPIRY_MS): Boolean {
        if (expiresAtMs <= 0) return false
        val window = if (issuedAtMs in 1 until expiresAtMs) minOf(windowMs, (expiresAtMs - issuedAtMs) / 2) else windowMs
        return expiresAtMs - nowMs <= window
    }
}
