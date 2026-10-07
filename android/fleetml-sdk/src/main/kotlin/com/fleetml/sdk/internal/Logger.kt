package com.fleetml.sdk.internal

import android.util.Log

internal object Logger {
    private const val TAG = "FleetML"

    fun d(msg: String) = Log.d(TAG, msg)
    fun i(msg: String) = Log.i(TAG, msg)
    fun w(msg: String) = Log.w(TAG, msg)
    fun w(msg: String, t: Throwable) = Log.w(TAG, msg, t)
    fun e(msg: String) = Log.e(TAG, msg)
    fun e(msg: String, t: Throwable) = Log.e(TAG, msg, t)
}
