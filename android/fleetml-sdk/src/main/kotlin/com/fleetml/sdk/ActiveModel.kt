package com.fleetml.sdk

import java.io.File

/**
 * A model the SDK has downloaded, verified (sha256) and validated, and that
 * the app should run. Pass it to [TelemetryReporter.reportInference] so events
 * carry the right model version (and experiment arm).
 */
data class ActiveModel(
    /** The model, stable across versions. */
    val modelId: String,
    val modelName: String,
    /** Human-readable version, e.g. "1.2.0". */
    val version: String,
    /** The model version ID; telemetry is attributed to it. */
    val versionId: String,
    val variantId: String,
    val sha256: String,
    /** The model file on disk. */
    val file: File,
)
