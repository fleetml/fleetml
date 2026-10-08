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
     * If already registered (token exists), re-registers to update hardware info,
     * authenticating with the current token. The enrollment key from [FleetMLConfig]
     * is always sent so a device whose token was revoked or expired can re-enroll.
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
            .setEnrollmentKey(config.enrollmentKey)
            .build()

        val currentToken = tokenManager.getAuthToken()
        if (currentToken == null && config.enrollmentKey.isBlank()) {
            Logger.w("No enrollment key configured: the server will reject the first registration")
        }
        val stub = currentToken?.let { grpcClient.authenticatedStub(it) } ?: grpcClient.deviceService
        val response = stub.register(request)
        tokenManager.saveRegistration(response.deviceId, response.authToken, response.tokenExpiresAt)

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
