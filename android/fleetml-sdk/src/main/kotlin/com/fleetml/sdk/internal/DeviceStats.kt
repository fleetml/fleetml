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

    fun batteryPercent(): Float = try {
        val status = context.registerReceiver(null, IntentFilter(Intent.ACTION_BATTERY_CHANGED))
        val level = status?.getIntExtra(BatteryManager.EXTRA_LEVEL, -1) ?: -1
        val scale = status?.getIntExtra(BatteryManager.EXTRA_SCALE, -1) ?: -1
        if (level >= 0 && scale > 0) level * 100f / scale else 0f
    } catch (_: Exception) {
        0f
    }
}
