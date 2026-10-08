package com.fleetml.sdk.internal

import android.content.Context
import android.content.SharedPreferences
import androidx.security.crypto.EncryptedSharedPreferences
import androidx.security.crypto.MasterKey

/**
 * Manages encrypted storage of device registration credentials.
 */
/** Where the device's identity and token are kept. */
internal interface TokenStore {
    fun getDeviceId(): String?
    fun getAuthToken(): String?
    fun getTokenExpiresAtMillis(): Long
    /** When the current token was obtained, in epoch millis (0 if unknown). */
    fun getTokenIssuedAtMillis(): Long = 0
    fun saveToken(authToken: String, tokenExpiresAtSeconds: Long)
}

internal class TokenManager(context: Context) : TokenStore {

    private val prefs: SharedPreferences by lazy {
        val masterKey = MasterKey.Builder(context)
            .setKeyScheme(MasterKey.KeyScheme.AES256_GCM)
            .build()

        EncryptedSharedPreferences.create(
            context,
            "fleetml_credentials",
            masterKey,
            EncryptedSharedPreferences.PrefKeyEncryptionScheme.AES256_SIV,
            EncryptedSharedPreferences.PrefValueEncryptionScheme.AES256_GCM
        )
    }

    fun saveRegistration(deviceId: String, authToken: String, tokenExpiresAtSeconds: Long, signature: String = "") {
        prefs.edit()
            .putString(KEY_DEVICE_ID, deviceId)
            .putString(KEY_AUTH_TOKEN, authToken)
            .putLong(KEY_TOKEN_EXPIRES_AT, tokenExpiresAtSeconds * 1000)
            .putLong(KEY_TOKEN_ISSUED_AT, System.currentTimeMillis())
            .putLong(KEY_REGISTERED_AT, System.currentTimeMillis())
            .putString(KEY_SIGNATURE, signature)
            .apply()
        Logger.d("Registration saved for device $deviceId")
    }

    /** Stores a refreshed token. The previous token stops working on the server. */
    override fun saveToken(authToken: String, tokenExpiresAtSeconds: Long) {
        prefs.edit()
            .putString(KEY_AUTH_TOKEN, authToken)
            .putLong(KEY_TOKEN_EXPIRES_AT, tokenExpiresAtSeconds * 1000)
            .putLong(KEY_TOKEN_ISSUED_AT, System.currentTimeMillis())
            .apply()
    }

    override fun getTokenIssuedAtMillis(): Long = prefs.getLong(KEY_TOKEN_ISSUED_AT, 0L)

    /** Token expiry in epoch millis, or 0 if unknown (registered by an older SDK). */
    override fun getTokenExpiresAtMillis(): Long = prefs.getLong(KEY_TOKEN_EXPIRES_AT, 0L)

    override fun getDeviceId(): String? = prefs.getString(KEY_DEVICE_ID, null)

    override fun getAuthToken(): String? = prefs.getString(KEY_AUTH_TOKEN, null)

    fun isRegistered(): Boolean = getDeviceId() != null && getAuthToken() != null

    /** What the device registered with (labels, SDK version); re-register when it changes. */
    fun getRegistrationSignature(): String = prefs.getString(KEY_SIGNATURE, "") ?: ""

    fun clear() {
        prefs.edit().clear().apply()
        Logger.d("Registration data cleared")
    }

    private companion object {
        const val KEY_DEVICE_ID = "device_id"
        const val KEY_AUTH_TOKEN = "auth_token"
        const val KEY_REGISTERED_AT = "registered_at"
        const val KEY_TOKEN_EXPIRES_AT = "token_expires_at"
        const val KEY_SIGNATURE = "registration_signature"
        const val KEY_TOKEN_ISSUED_AT = "token_issued_at"
    }
}
