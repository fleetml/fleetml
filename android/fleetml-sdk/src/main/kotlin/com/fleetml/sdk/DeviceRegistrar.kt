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
        tokenManager.saveRegistration(response.deviceId, response.authToken, response.tokenExpiresAt, signature())

        Logger.i("Registered as device ${response.deviceId}")
        return RegisterResult(
            deviceId = response.deviceId,
            heartbeatIntervalSeconds = response.config.heartbeatIntervalSeconds.toInt(),
            telemetryBatchSize = response.config.telemetryBatchSize.toInt(),
            maxCacheSizeMb = response.config.maxCacheSizeMb.toInt(),
            telemetryFlushIntervalSeconds = response.config.telemetryFlushIntervalSeconds.toInt(),
            config = response.config,
            signingKeys = response.signingKeysList,
        )
    }

    /**
     * True if the device must (re-)register: never registered, or the SDK
     * version, the labels, the server or the enrollment key changed (a build
     * with another project's key moves the device there; the server allows it
     * because the device also presents its current token).
     */
    fun needsRegistration(): Boolean = !tokenManager.isRegistered() || tokenManager.getRegistrationSignature() != signature()

    private fun signature(): String =
        registrationSignature(BuildConfig.SDK_VERSION, config.labels, config.serverHost, config.serverPort, config.enrollmentKey)

    companion object {
        /**
         * What a registration was made with. The key is kept as a SHA-256 digest
         * only. A signature saved by an older SDK (version and labels only)
         * differs, so such a device registers once more, with its token.
         */
        internal fun registrationSignature(sdkVersion: String, labels: Map<String, String>, host: String, port: Int, enrollmentKey: String): String {
            val key = java.security.MessageDigest.getInstance("SHA-256").digest(enrollmentKey.trim().toByteArray())
                .joinToString("") { "%02x".format(it) }
            return sdkVersion + "|" + labels.toSortedMap().entries.joinToString(",") { "${it.key}=${it.value}" } + "|" + host + ":" + port + "|" + key
        }
    }
}

/** Result of a successful device registration. */
data class RegisterResult(
    val deviceId: String,
    val heartbeatIntervalSeconds: Int,
    val telemetryBatchSize: Int,
    val maxCacheSizeMb: Int,
    val telemetryFlushIntervalSeconds: Int = 0,
    val config: fleetml.device.v1.Device.DeviceConfig = fleetml.device.v1.Device.DeviceConfig.getDefaultInstance(),
    /** The project's artifact signing keys. */
    val signingKeys: List<fleetml.device.v1.Device.SigningKey> = emptyList(),
)
