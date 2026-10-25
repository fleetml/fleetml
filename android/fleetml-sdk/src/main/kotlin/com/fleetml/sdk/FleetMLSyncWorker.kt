package com.fleetml.sdk

import android.content.Context
import androidx.work.CoroutineWorker
import androidx.work.WorkerParameters
import com.fleetml.sdk.internal.Logger
import io.grpc.Status
import io.grpc.StatusException

/**
 * The background check: one heartbeat and one telemetry flush. Scheduled by the
 * SDK when the app goes to the background; the app's `Application.onCreate()`
 * initializes FleetML before WorkManager runs it.
 */
class FleetMLSyncWorker(context: Context, params: WorkerParameters) : CoroutineWorker(context, params) {
    override suspend fun doWork(): Result {
        if (!FleetML.isInitialized()) return Result.success()
        val reached = FleetML.syncNow()
        Logger.i("Background sync: ${describeSync(reached, FleetML.lastSyncFailure())}")
        FleetML.scheduleBackgroundSync() // the next run retries; no extra wake-ups
        return Result.success()
    }
}

/**
 * The background job's log line. A gRPC status from the server (other than
 * UNAVAILABLE or DEADLINE_EXCEEDED, which the client reports when it gets no
 * answer) means the server was reached and refused the request, for example
 * a revoked enrollment key; anything else means it was not reached.
 */
internal fun describeSync(reached: Boolean, failure: Throwable?): String {
    if (reached) return "server reached"
    val status = (failure as? StatusException)?.status
    return when (status?.code) {
        null, Status.Code.UNAVAILABLE, Status.Code.DEADLINE_EXCEEDED -> "server unreachable"
        else -> "server reached, request refused (${status.code}: ${status.description.orEmpty()})"
    }
}
