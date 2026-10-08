package com.fleetml.sdk.internal

import android.app.ActivityManager
import android.content.Context
import android.content.Intent
import android.content.IntentFilter
import android.os.BatteryManager

/** Device readings attached to inference telemetry. */
internal class DeviceStats(private val context: Context) {
    fun availableMemoryMb(): Int = try {
        val am = context.getSystemService(Context.ACTIVITY_SERVICE) as ActivityManager
        val info = ActivityManager.MemoryInfo()
        am.getMemoryInfo(info)
        (info.availMem / (1024 * 1024)).toInt()
    } catch (_: Exception) {
        0
    }

    /** True on a metered network (mobile data). */
    fun isNetworkMetered(): Boolean = try {
        (context.getSystemService(Context.CONNECTIVITY_SERVICE) as android.net.ConnectivityManager).isActiveNetworkMetered
    } catch (_: Exception) {
        false
    }

    /** True if a network with internet access is connected. */
    fun isConnected(): Boolean = try {
        val cm = context.getSystemService(Context.CONNECTIVITY_SERVICE) as android.net.ConnectivityManager
        cm.getNetworkCapabilities(cm.activeNetwork)?.hasCapability(android.net.NetworkCapabilities.NET_CAPABILITY_INTERNET) == true
    } catch (_: Exception) {
        false
    }

    /** True while the device is charging or full on a charger. */
    fun isCharging(): Boolean = try {
        val status = context.registerReceiver(null, IntentFilter(Intent.ACTION_BATTERY_CHANGED))
        when (status?.getIntExtra(BatteryManager.EXTRA_STATUS, -1)) {
            BatteryManager.BATTERY_STATUS_CHARGING, BatteryManager.BATTERY_STATUS_FULL -> true
            else -> false
        }
    } catch (_: Exception) {
        false
    }

    fun batteryPercent(): Float = try {
        val status = context.registerReceiver(null, IntentFilter(Intent.ACTION_BATTERY_CHANGED))
        val level = status?.getIntExtra(BatteryManager.EXTRA_LEVEL, -1) ?: -1
        val scale = status?.getIntExtra(BatteryManager.EXTRA_SCALE, -1) ?: -1
        if (level >= 0 && scale > 0) level * 100f / scale else 0f
    } catch (_: Exception) {
        0f
    }
}
