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
 * @param heartbeatIntervalSeconds Interval between heartbeats while the app is in the foreground.
 * @param backgroundHeartbeatIntervalSeconds Interval of the background check (one heartbeat and one
 *   telemetry flush through WorkManager) while the app is in the background; at least 15 minutes.
 * @param maxCacheSizeMb Maximum disk cache size for downloaded models in MB.
 * @param labels Custom device labels sent to the server (e.g., environment, region).
 * @param downloadPolicy Which networks non-urgent model downloads may use; assignments wait until
 *   then. Models already cached are activated regardless.
 * @param autoStart If true, registration and heartbeat start immediately on init.
 * @param telemetryBatchSize Events per telemetry request (the server's DeviceConfig overrides it).
 * @param telemetryFlushIntervalSeconds How often queued telemetry is sent (the server's DeviceConfig overrides it).
 */
data class FleetMLConfig(
    val serverHost: String,
    val serverPort: Int = 19090,
    val enrollmentKey: String = "",
    val useTls: Boolean = false,
    val heartbeatIntervalSeconds: Long = 30,
    val backgroundHeartbeatIntervalSeconds: Long = 3600,
    val maxCacheSizeMb: Long = 500,
    val labels: Map<String, String> = emptyMap(),
    val downloadPolicy: DownloadPolicy = DownloadPolicy.UNMETERED_OR_CHARGING,
    val autoStart: Boolean = true,
    val telemetryBatchSize: Int = 50,
    val telemetryFlushIntervalSeconds: Long = 30,
)
