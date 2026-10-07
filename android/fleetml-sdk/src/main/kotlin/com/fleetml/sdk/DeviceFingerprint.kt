package com.fleetml.sdk

import android.app.ActivityManager
import android.content.Context
import android.os.Build
import java.security.MessageDigest

/**
 * Generates a stable, unique SHA256 fingerprint from device hardware.
 *
 * The fingerprint is deterministic: the same physical device always produces
 * the same fingerprint. No user-resettable or privacy-sensitive identifiers
 * (ANDROID_ID, IMEI, etc.) are used.
 */
object DeviceFingerprint {

    /**
     * Generate a 64-character lowercase hex SHA256 fingerprint.
     */
    fun generate(context: Context): String {
        val parts = listOf(
            Build.BOARD.orEmpty(),
            Build.HARDWARE.orEmpty(),
            getSocManufacturer(),
            getSocModel(),
            Runtime.getRuntime().availableProcessors().toString(),
            getMemoryClass(context).toString()
        )
        val data = parts.joinToString(":")
        val digest = MessageDigest.getInstance("SHA-256")
        val hash = digest.digest(data.toByteArray())
        return hash.joinToString("") { "%02x".format(it) }
    }

    fun getOS(): String = "ANDROID"

    fun getOSVersion(): String = Build.VERSION.RELEASE

    fun getChipset(): String = getSocModel().ifEmpty { Build.HARDWARE.orEmpty() }

    fun getRAMMB(context: Context): Int {
        val am = context.getSystemService(Context.ACTIVITY_SERVICE) as ActivityManager
        val memInfo = ActivityManager.MemoryInfo()
        am.getMemoryInfo(memInfo)
        return (memInfo.totalMem / (1024 * 1024)).toInt()
    }

    fun hasNPU(): Boolean {
        // NNAPI is available on Android 8.1+ (API 27+)
        return Build.VERSION.SDK_INT >= 27
    }

    fun hasGPU(): Boolean {
        // GPU is available on all modern Android devices
        return true
    }

    private fun getSocManufacturer(): String {
        return if (Build.VERSION.SDK_INT >= 31) {
            Build.SOC_MANUFACTURER.orEmpty()
        } else {
            Build.MANUFACTURER.orEmpty()
        }
    }

    private fun getSocModel(): String {
        return if (Build.VERSION.SDK_INT >= 31) {
            Build.SOC_MODEL.orEmpty()
        } else {
            Build.HARDWARE.orEmpty()
        }
    }

    private fun getMemoryClass(context: Context): Int {
        val am = context.getSystemService(Context.ACTIVITY_SERVICE) as ActivityManager
        return am.memoryClass
    }
}
