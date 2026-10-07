package com.fleetml.sdk

import android.app.ActivityManager
import android.content.Context
import io.mockk.every
import io.mockk.mockk
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class DeviceFingerprintTest {

    @Test
    fun `fingerprint is 64 character hex string`() {
        val context = createMockContext()
        val fp = DeviceFingerprint.generate(context)
        assertEquals(64, fp.length)
        assertTrue(fp.matches(Regex("[a-f0-9]{64}")))
    }

    @Test
    fun `fingerprint is deterministic`() {
        val context = createMockContext()
        val fp1 = DeviceFingerprint.generate(context)
        val fp2 = DeviceFingerprint.generate(context)
        assertEquals(fp1, fp2)
    }

    @Test
    fun `getOS returns ANDROID`() {
        assertEquals("ANDROID", DeviceFingerprint.getOS())
    }

    @Test
    fun `hasGPU returns true`() {
        assertTrue(DeviceFingerprint.hasGPU())
    }

    private fun createMockContext(): Context {
        val memInfo = ActivityManager.MemoryInfo().apply {
            totalMem = 8L * 1024 * 1024 * 1024 // 8GB
        }
        val activityManager = mockk<ActivityManager> {
            every { getMemoryInfo(any()) } answers {
                val info = firstArg<ActivityManager.MemoryInfo>()
                info.totalMem = memInfo.totalMem
            }
            every { memoryClass } returns 256
        }
        return mockk<Context> {
            every { getSystemService(Context.ACTIVITY_SERVICE) } returns activityManager
        }
    }
}
