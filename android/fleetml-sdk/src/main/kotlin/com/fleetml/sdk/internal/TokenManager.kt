package com.fleetml.sdk.internal

import android.content.Context
import android.content.SharedPreferences
import androidx.security.crypto.EncryptedSharedPreferences
import androidx.security.crypto.MasterKey
import java.io.IOException
import java.security.GeneralSecurityException
import java.security.KeyStore

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

internal class TokenManager(
    context: Context,
    private val openStore: () -> SharedPreferences = { openEncrypted(context) },
    private val discardStore: (withKey: Boolean) -> Unit = { withKey -> discardEncrypted(context, withKey) },
) : TokenStore {

    @Volatile private var store: SharedPreferences? = null

    /**
     * The encrypted store. When it cannot be decrypted (restored from a backup
     * or copied from another install, so the Keystore key that wrapped it is
     * gone), it is dropped and the device registers again with its enrollment
     * key; the device ID is the hardware fingerprint, so it comes back the same.
     */
    private val prefs: SharedPreferences
        get() = store ?: synchronized(this) { store ?: open().also { store = it } }

    private fun open(): SharedPreferences =
        try {
            openStore()
        } catch (e: Exception) {
            if (!unreadable(e)) throw e
            recovered(e)
            discardStore(false)
            try {
                openStore()
            } catch (again: Exception) {
                if (!unreadable(again)) throw again
                // The file was not the problem: the key itself is unusable.
                discardStore(true)
                openStore()
            }
        }

    /** A read whose value cannot be decrypted is the same case as an unreadable file. */
    private inline fun <T> read(default: T, get: SharedPreferences.() -> T): T =
        try {
            prefs.get()
        } catch (e: SecurityException) {
            synchronized(this) {
                recovered(e)
                store = null
                discardStore(false)
            }
            default
        }

    // One line: Keystore messages run over several lines; the trace stays at debug.
    private fun recovered(e: Exception) {
        Logger.w("Stored registration unreadable (${e.javaClass.simpleName}); deleted it, registering again with the enrollment key")
        Logger.d("Stored registration unreadable", e)
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

    override fun getTokenIssuedAtMillis(): Long = read(0L) { getLong(KEY_TOKEN_ISSUED_AT, 0L) }

    /** Token expiry in epoch millis, or 0 if unknown (registered by an older SDK). */
    override fun getTokenExpiresAtMillis(): Long = read(0L) { getLong(KEY_TOKEN_EXPIRES_AT, 0L) }

    override fun getDeviceId(): String? = read(null) { getString(KEY_DEVICE_ID, null) }

    override fun getAuthToken(): String? = read(null) { getString(KEY_AUTH_TOKEN, null) }

    fun isRegistered(): Boolean = getDeviceId() != null && getAuthToken() != null

    /** What the device registered with (labels, SDK version); re-register when it changes. */
    fun getRegistrationSignature(): String = read("") { getString(KEY_SIGNATURE, "") ?: "" }

    fun clear() {
        prefs.edit().clear().apply()
        Logger.d("Registration data cleared")
    }

    private companion object {
        const val FILE = "fleetml_credentials"

        fun openEncrypted(context: Context): SharedPreferences =
            EncryptedSharedPreferences.create(
                context,
                FILE,
                MasterKey.Builder(context).setKeyScheme(MasterKey.KeyScheme.AES256_GCM).build(),
                EncryptedSharedPreferences.PrefKeyEncryptionScheme.AES256_SIV,
                EncryptedSharedPreferences.PrefValueEncryptionScheme.AES256_GCM,
            )

        /**
         * Deletes the credentials file (it holds its own data keys, wrapped by
         * the master key). The master key is shared with any other
         * EncryptedSharedPreferences of the app, so it is deleted only when it
         * is itself unusable.
         */
        fun discardEncrypted(context: Context, withKey: Boolean) {
            context.deleteSharedPreferences(FILE)
            if (withKey) {
                KeyStore.getInstance("AndroidKeyStore").apply { load(null) }
                    .deleteEntry(MasterKey.DEFAULT_MASTER_KEY_ALIAS)
            }
        }

        /** Keystore and Tink failures, directly or as the cause of a wrapper. */
        fun unreadable(e: Throwable): Boolean =
            generateSequence(e) { it.cause.takeIf { c -> c !== it } }
                .any { it is GeneralSecurityException || it is IOException || it is SecurityException }

        const val KEY_DEVICE_ID = "device_id"
        const val KEY_AUTH_TOKEN = "auth_token"
        const val KEY_REGISTERED_AT = "registered_at"
        const val KEY_TOKEN_EXPIRES_AT = "token_expires_at"
        const val KEY_SIGNATURE = "registration_signature"
        const val KEY_TOKEN_ISSUED_AT = "token_issued_at"
    }
}
