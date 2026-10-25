package com.fleetml.sdk

import android.content.Context
import android.content.SharedPreferences
import androidx.test.core.app.ApplicationProvider
import com.fleetml.sdk.internal.Logger
import com.fleetml.sdk.internal.TokenManager
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import java.lang.reflect.Proxy
import java.security.KeyStoreException
import javax.crypto.AEADBadTagException

// The Keystore is not available under Robolectric, so the encrypted store is
// replaced by plain preferences; androidTest TokenManagerCorruptFileTest runs
// the same recovery on a real corrupted file.
@RunWith(RobolectricTestRunner::class)
class TokenManagerRecoveryTest {
    private val context: Context = ApplicationProvider.getApplicationContext()
    private val warnings = mutableListOf<String>()
    private val discarded = mutableListOf<Boolean>()
    private val saved = Logger.sink

    @Before
    fun setUp() {
        Logger.sink = { p, m, _ -> if (p >= android.util.Log.WARN) warnings += m }
    }

    @After
    fun tearDown() {
        Logger.sink = saved
    }

    private fun plain(): SharedPreferences = context.getSharedPreferences("recovery-test", Context.MODE_PRIVATE)

    private fun manager(opens: MutableList<() -> SharedPreferences>) =
        TokenManager(context, openStore = { opens.removeAt(0)() }, discardStore = { discarded += it })

    @Test
    fun `an undecryptable store is deleted and the device is unregistered`() {
        val tm = manager(mutableListOf<() -> SharedPreferences>({ throw AEADBadTagException() }, ::plain))
        assertNull(tm.getDeviceId())
        assertFalse(tm.isRegistered())
        assertEquals(listOf(false), discarded)
        assertEquals(1, warnings.size)
        assertTrue(warnings[0], warnings[0].startsWith("Stored registration unreadable (AEADBadTagException)"))
        assertTrue(warnings[0], warnings[0].endsWith("registering again with the enrollment key"))

        tm.saveRegistration("dev-1", "tok", 2_000_000_000L)
        assertEquals("dev-1", tm.getDeviceId())
        assertEquals("one warning, not one per read", 1, warnings.size)
    }

    @Test
    fun `an unusable master key is deleted only when the file was not the problem`() {
        val keystore = KeyStoreException("Signature/MAC verification failed")
        val tm = manager(mutableListOf<() -> SharedPreferences>({ throw AEADBadTagException() }, { throw keystore }, ::plain))
        assertNull(tm.getDeviceId())
        assertEquals(listOf(false, true), discarded)
    }

    @Test
    fun `a value that cannot be decrypted is the same case`() {
        val failing = Proxy.newProxyInstance(SharedPreferences::class.java.classLoader, arrayOf(SharedPreferences::class.java)) { _, method, _ ->
            if (method.name == "getString") throw SecurityException("Could not decrypt value. decryption failed")
            null
        } as SharedPreferences
        val tm = manager(mutableListOf<() -> SharedPreferences>({ failing }, ::plain))
        assertNull(tm.getDeviceId())
        assertEquals(listOf(false), discarded)
        assertTrue(warnings.single().startsWith("Stored registration unreadable (SecurityException)"))
        assertNull("the next read opens a fresh store", tm.getAuthToken())
    }

    @Test(expected = IllegalStateException::class)
    fun `other failures are not swallowed`() {
        manager(mutableListOf<() -> SharedPreferences>({ throw IllegalStateException("not a storage problem") })).getDeviceId()
    }
}
