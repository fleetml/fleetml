package com.fleetml.sdk.internal

import android.content.Context
import android.content.SharedPreferences
import androidx.security.crypto.EncryptedSharedPreferences
import androidx.security.crypto.MasterKey

/**
 * Manages encrypted storage of device registration credentials.
 */
internal class TokenManager(context: Context) {

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

    fun saveRegistration(deviceId: String, authToken: String, tokenExpiresAtSeconds: Long) {
        prefs.edit()
            .putString(KEY_DEVICE_ID, deviceId)
            .putString(KEY_AUTH_TOKEN, authToken)
            .putLong(KEY_TOKEN_EXPIRES_AT, tokenExpiresAtSeconds * 1000)
            .putLong(KEY_REGISTERED_AT, System.currentTimeMillis())
            .apply()
        Logger.d("Registration saved for device $deviceId")
    }

    /** Stores a refreshed token. The previous token stops working on the server. */
    fun saveToken(authToken: String, tokenExpiresAtSeconds: Long) {
        prefs.edit()
            .putString(KEY_AUTH_TOKEN, authToken)
            .putLong(KEY_TOKEN_EXPIRES_AT, tokenExpiresAtSeconds * 1000)
            .apply()
    }

    /** Token expiry in epoch millis, or 0 if unknown (registered by an older SDK). */
    fun getTokenExpiresAtMillis(): Long = prefs.getLong(KEY_TOKEN_EXPIRES_AT, 0L)

    fun getDeviceId(): String? = prefs.getString(KEY_DEVICE_ID, null)

    fun getAuthToken(): String? = prefs.getString(KEY_AUTH_TOKEN, null)

    fun isRegistered(): Boolean = getDeviceId() != null && getAuthToken() != null

    fun clear() {
        prefs.edit().clear().apply()
        Logger.d("Registration data cleared")
    }

    private companion object {
        const val KEY_DEVICE_ID = "device_id"
        const val KEY_AUTH_TOKEN = "auth_token"
        const val KEY_REGISTERED_AT = "registered_at"
        const val KEY_TOKEN_EXPIRES_AT = "token_expires_at"
    }
}
