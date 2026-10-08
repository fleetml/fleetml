package com.fleetml.sdk

/**
 * Lifecycle events from the SDK. Every method has an empty default, so
 * implement only what the app needs. Callbacks run on a background thread.
 */
interface FleetMLCallback {
    /**
     * A model was downloaded, verified (sha256) and validated, and is now the
     * active version of its model: load [ActiveModel.file] and use it. Also
     * called at start for models cached from a previous run (offline start).
     */
    fun onModelReady(model: ActiveModel) {}

    /** The server no longer assigns this model (e.g. its deployment was rolled back with nothing to return to). */
    fun onModelUnload(modelId: String, modelName: String, variantId: String) {}

    /**
     * A new version could not be downloaded or failed validation. The
     * previously active (last-known-good) version stays active.
     */
    fun onModelLoadFailed(modelName: String, version: String, reason: String) {}

    /** The device registered (or re-registered) with the server. */
    fun onRegistered(deviceId: String) {}

    /** The server became reachable or unreachable. */
    fun onConnectionStatusChanged(connected: Boolean) {}
}
