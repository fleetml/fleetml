package com.fleetml.sdk.internal

/**
 * Decides when to refresh the device token.
 *
 * The server accepts RefreshToken in the last 7 days of a token's life. The SDK
 * asks one day later than that (6 days before expiry) so a device clock that runs
 * up to a day fast still lands inside the server's window.
 */
internal object TokenRefreshPolicy {
    const val REFRESH_BEFORE_EXPIRY_MS: Long = 6L * 24 * 60 * 60 * 1000

    /** True when a known expiry is close enough to refresh. Unknown expiry (0) never refreshes. */
    fun needsRefresh(nowMs: Long, expiresAtMs: Long, windowMs: Long = REFRESH_BEFORE_EXPIRY_MS): Boolean =
        expiresAtMs > 0 && expiresAtMs - nowMs <= windowMs
}
