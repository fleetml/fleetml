package com.fleetml.sdk

import android.content.Context
import androidx.work.CoroutineWorker
import androidx.work.WorkerParameters
import com.fleetml.sdk.internal.Logger

/**
 * The background check: one heartbeat and one telemetry flush. Scheduled by the
 * SDK when the app goes to the background; the app's `Application.onCreate()`
 * initializes FleetML before WorkManager runs it.
 */
class FleetMLSyncWorker(context: Context, params: WorkerParameters) : CoroutineWorker(context, params) {
    override suspend fun doWork(): Result {
        if (!FleetML.isInitialized()) return Result.success()
        val reached = FleetML.syncNow()
        Logger.i("Background sync: server ${if (reached) "reached" else "unreachable"}")
        FleetML.scheduleBackgroundSync() // the next run retries; no extra wake-ups
        return Result.success()
    }
}
