package com.fleetml.sdk

/**
 * Configuration for the FleetML Android SDK.
 *
 * @param serverHost The FleetML server hostname or IP address.
 * @param serverPort The gRPC port on the FleetML server.
 * @param enrollmentKey Per-project enrollment key (create one with
 *   `POST /api/v1/projects/{id}/enrollment-keys`). Required for the first registration;
 *   afterwards the device re-registers with its own token.
 * @param useTls Whether to use TLS for gRPC connections.
 * @param heartbeatIntervalSeconds Interval between heartbeats in seconds.
 * @param maxCacheSizeMb Maximum disk cache size for downloaded models in MB.
 * @param labels Custom device labels sent to the server (e.g., environment, region).
 * @param downloadOnWifiOnly If true, model downloads are restricted to WiFi connections.
 * @param autoStart If true, registration and heartbeat start immediately on init.
 */
data class FleetMLConfig(
    val serverHost: String,
    val serverPort: Int = 19090,
    val enrollmentKey: String = "",
    val useTls: Boolean = false,
    val heartbeatIntervalSeconds: Long = 30,
    val maxCacheSizeMb: Long = 500,
    val labels: Map<String, String> = emptyMap(),
    val downloadOnWifiOnly: Boolean = true,
    val autoStart: Boolean = true,
    val telemetryBatchSize: Int = 50,
)
