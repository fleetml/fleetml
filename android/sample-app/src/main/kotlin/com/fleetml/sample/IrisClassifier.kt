package com.fleetml.sample

import android.content.Context
import org.tensorflow.lite.Interpreter
import java.io.File
import java.io.FileOutputStream

/** Runs the Iris TFLite model: 4 measurements in, 3 class probabilities out. */
class IrisClassifier(private val modelFile: File) {
    /** [latencyMicros] is measured; [latencyMs] is what telemetry gets (at least 1 ms). */
    data class Result(val label: String, val confidence: Float, val latencyMicros: Long) {
        val latencyMs: Int get() = ((latencyMicros + 999) / 1000).toInt().coerceAtLeast(1)
    }

    fun classify(measurements: FloatArray): Result {
        Interpreter(modelFile).use { interpreter ->
            val input = arrayOf(measurements)
            val output = arrayOf(FloatArray(LABELS.size))
            val start = System.nanoTime()
            interpreter.run(input, output)
            val latencyMicros = (System.nanoTime() - start) / 1_000
            val best = output[0].indices.maxByOrNull { output[0][it] } ?: 0
            return Result(LABELS[best], output[0][best], latencyMicros)
        }
    }

    companion object {
        val LABELS = listOf("setosa", "versicolor", "virginica")

        /** A flower measurement (cm) bundled with the app: sepal length, sepal width, petal length, petal width. */
        val SAMPLE_INPUT = floatArrayOf(6.7f, 3.0f, 5.2f, 2.3f)

        /** The model shipped in the APK, used until the server assigns one. */
        fun bundledModel(context: Context): File {
            val file = File(context.filesDir, "iris_bundled.tflite")
            if (!file.exists()) {
                context.assets.open("iris_bundled.tflite").use { input -> FileOutputStream(file).use { input.copyTo(it) } }
            }
            return file
        }
    }
}
