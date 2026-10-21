package com.fleetml.sdk.internal

import android.util.Log
import io.grpc.StatusException
import io.grpc.StatusRuntimeException

internal object Logger {
    private const val TAG = "FleetML"

    /** Where the lines go: logcat, or a recorder in tests. */
    @Volatile
    var sink: (priority: Int, msg: String, t: Throwable?) -> Unit = { priority, msg, t ->
        when (priority) {
            Log.DEBUG -> Log.d(TAG, msg, t)
            Log.INFO -> Log.i(TAG, msg, t)
            Log.WARN -> Log.w(TAG, msg, t)
            else -> Log.e(TAG, msg, t)
        }
    }

    fun d(msg: String) = sink(Log.DEBUG, msg, null)
    fun d(msg: String, t: Throwable) = sink(Log.DEBUG, msg, t)
    fun i(msg: String) = sink(Log.INFO, msg, null)
    fun w(msg: String) = sink(Log.WARN, msg, null)
    fun e(msg: String) = sink(Log.ERROR, msg, null)

    /**
     * A failed call: one line at warning (or error) level with the gRPC status
     * and the underlying cause, and the stack trace at debug level only. A
     * device offline for an hour would otherwise fill logcat with traces.
     */
    fun failure(msg: String, t: Throwable, priority: Int = Log.WARN) {
        sink(priority, "$msg: ${describe(t)}", null)
        sink(Log.DEBUG, "$msg (stack trace)", t)
    }

    /** `UNAVAILABLE: <description>: <root cause>` for gRPC errors, else the message. */
    fun describe(t: Throwable): String {
        val status = when (t) {
            is StatusException -> t.status
            is StatusRuntimeException -> t.status
            else -> null
        }
        var root: Throwable = status?.cause ?: t
        while (root.cause != null && root.cause !== root) root = root.cause!!
        val rootMessage = root.message ?: root.javaClass.simpleName
        if (status == null) {
            val own = t.message ?: t.javaClass.simpleName
            return if (root === t || own.contains(rootMessage)) own else "$own: $rootMessage"
        }
        val parts = listOfNotNull(status.description, if (status.cause != null) rootMessage else null).distinct()
        return (listOf(status.code.name) + parts).joinToString(": ")
    }
}
