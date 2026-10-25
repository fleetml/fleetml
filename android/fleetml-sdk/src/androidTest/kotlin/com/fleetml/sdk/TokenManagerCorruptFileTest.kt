package com.fleetml.sdk

import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import com.fleetml.sdk.internal.TokenManager
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test
import org.junit.runner.RunWith
import java.io.File

/**
 * A credentials file whose data keys no longer decrypt (what a backup restore
 * or a copied app directory leaves) is replaced instead of crashing the app.
 */
@RunWith(AndroidJUnit4::class)
class TokenManagerCorruptFileTest {
    private val context = InstrumentationRegistry.getInstrumentation().targetContext

    @Test
    fun aCorruptedCredentialsFileIsReplaced() {
        context.deleteSharedPreferences("fleetml_credentials")
        File(context.dataDir, "shared_prefs").mkdirs()
        File(context.dataDir, "shared_prefs/fleetml_credentials.xml").writeText(
            """<?xml version='1.0' encoding='utf-8' standalone='yes' ?>
            |<map>
            |    <string name="__androidx_security_crypto_encrypted_prefs_key_keyset__">12a901003d1c6c10c1b2d0a0b1</string>
            |    <string name="__androidx_security_crypto_encrypted_prefs_value_keyset__">12a901003d1c6c10c1b2d0a0b1</string>
            |</map>
            |""".trimMargin(),
        )

        val tm = TokenManager(context)
        assertNull(tm.getDeviceId())

        tm.saveRegistration("dev-1", "tok", 2_000_000_000L)
        assertEquals("dev-1", TokenManager(context).getDeviceId())
        context.deleteSharedPreferences("fleetml_credentials")
    }
}
