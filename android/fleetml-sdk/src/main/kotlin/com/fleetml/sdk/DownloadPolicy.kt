package com.fleetml.sdk

import fleetml.device.v1.Device

/**
 * When the SDK may download a model that is not marked urgent. The server's
 * DeviceConfig overrides the value in [FleetMLConfig]; urgent assignments
 * download on any connected network.
 */
enum class DownloadPolicy {
    /** An unmetered network (Wi-Fi, Ethernet), or any network while charging. */
    UNMETERED_OR_CHARGING,

    /** Only an unmetered network. */
    UNMETERED_ONLY,

    /** Any connected network. */
    ANY_NETWORK,
}

/** The battery and data rules, as pure functions so they are easy to test. */
internal object BatteryPolicy {
    /** Android runs periodic background work at most every 15 minutes. */
    const val MIN_BACKGROUND_INTERVAL_SECONDS = 15 * 60L

    /** Whether a model download may start now. */
    fun mayDownload(policy: DownloadPolicy, urgent: Boolean, connected: Boolean, metered: Boolean, charging: Boolean): Boolean {
        if (!connected) return false
        if (urgent) return true
        return when (policy) {
            DownloadPolicy.ANY_NETWORK -> true
            DownloadPolicy.UNMETERED_ONLY -> !metered
            DownloadPolicy.UNMETERED_OR_CHARGING -> !metered || charging
        }
    }

    /** The background check interval: the server's, but never below the OS minimum. */
    fun backgroundIntervalSeconds(requested: Long): Long = maxOf(MIN_BACKGROUND_INTERVAL_SECONDS, requested)

    /** The server's policy, or [fallback] when the server leaves it unspecified. */
    fun fromProto(policy: Device.DownloadPolicy, fallback: DownloadPolicy): DownloadPolicy = when (policy) {
        Device.DownloadPolicy.UNMETERED_OR_CHARGING -> DownloadPolicy.UNMETERED_OR_CHARGING
        Device.DownloadPolicy.UNMETERED_ONLY -> DownloadPolicy.UNMETERED_ONLY
        Device.DownloadPolicy.ANY_NETWORK -> DownloadPolicy.ANY_NETWORK
        else -> fallback
    }
}
