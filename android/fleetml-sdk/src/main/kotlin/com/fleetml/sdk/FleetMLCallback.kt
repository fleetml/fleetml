package com.fleetml.sdk

import java.io.File

/**
 * Callback interface for app developers to receive FleetML lifecycle events.
 *
 * Implement this interface and pass it to [FleetML.init] to receive notifications
 * about model readiness, downloads, and connection status.
 */
interface FleetMLCallback {
    /**
     * Called when a new model is available on disk, ready to load.
     *
     * @param modelName The model name (e.g., "fraud-detector")
     * @param version The semver version (e.g., "1.2.0")
     * @param variantId The specific variant ID
     * @param modelFile The local File path to the downloaded model
     */
    fun onModelReady(modelName: String, version: String, variantId: String, modelFile: File)

    /**
     * Called when a model should be unloaded (e.g., deployment was rolled back).
     */
    fun onModelUnload(modelName: String, variantId: String)

    /**
     * Called when model download fails.
     */
    fun onModelDownloadFailed(modelName: String, version: String, reason: String)

    /**
     * Called when device registration completes.
     */
    fun onRegistered(deviceId: String)

    /**
     * Called when connection to FleetML server is lost or restored.
     */
    fun onConnectionStatusChanged(connected: Boolean)
}
