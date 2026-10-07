package com.fleetml.sdk

import android.content.Context
import com.fleetml.sdk.internal.GrpcClient
import com.fleetml.sdk.internal.Logger
import com.fleetml.sdk.internal.TokenManager
import fleetml.device.v1.Device.RegisterRequest

/**
 * Handles device registration with the FleetML server.
 */
internal class DeviceRegistrar(
    private val grpcClient: GrpcClient,
    private val tokenManager: TokenManager,
    private val config: FleetMLConfig,
    private val context: Context
) {
    /**
     * Register this device with FleetML server.
     * If already registered (token exists), re-registers to update hardware info.
     * Returns the server-assigned configuration.
     */
    suspend fun register(): RegisterResult {
        val fingerprint = DeviceFingerprint.generate(context)
        Logger.d("Registering device with fingerprint ${fingerprint.take(12)}...")

        val request = RegisterRequest.newBuilder()
            .setFingerprint(fingerprint)
            .setOs(DeviceFingerprint.getOS())
            .setOsVersion(DeviceFingerprint.getOSVersion())
            .setChipset(DeviceFingerprint.getChipset())
            .setRamMb(DeviceFingerprint.getRAMMB(context))
            .setHasNpu(DeviceFingerprint.hasNPU())
            .setHasGpu(DeviceFingerprint.hasGPU())
            .setAgentVersion(BuildConfig.SDK_VERSION)
            .putAllLabels(config.labels)
            .build()

        val response = grpcClient.deviceService.register(request)
        tokenManager.saveRegistration(response.deviceId, response.authToken)

        Logger.i("Registered as device ${response.deviceId}")
        return RegisterResult(
            deviceId = response.deviceId,
            heartbeatIntervalSeconds = response.config.heartbeatIntervalSeconds.toInt(),
            telemetryBatchSize = response.config.telemetryBatchSize.toInt(),
            maxCacheSizeMb = response.config.maxCacheSizeMb.toInt()
        )
    }
}

/** Result of a successful device registration. */
data class RegisterResult(
    val deviceId: String,
    val heartbeatIntervalSeconds: Int,
    val telemetryBatchSize: Int,
    val maxCacheSizeMb: Int
)
