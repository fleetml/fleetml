package com.fleetml.sdk

import com.fleetml.sdk.internal.Logger
import java.io.File
import java.nio.ByteBuffer
import java.nio.ByteOrder

/**
 * Checks that a downloaded, sha256-verified model actually loads and runs on
 * this device before it becomes the active model. If validation fails the
 * previous (last-known-good) model stays active and the server is told
 * MODEL_LOAD_FAILED.
 *
 * Pass your own to [FleetML.init] to check what your app needs (input
 * shapes, a golden input, a latency budget).
 */
fun interface ModelValidator {
    fun validate(file: File, model: ActiveModel): ValidationResult
}

sealed class ValidationResult {
    object Valid : ValidationResult()
    data class Invalid(val reason: String) : ValidationResult()
}

/** Accepts every model. Use it when the app validates models itself. */
object NoOpModelValidator : ModelValidator {
    override fun validate(file: File, model: ActiveModel) = ValidationResult.Valid
}

/**
 * Opens the model with the TensorFlow Lite Interpreter and runs one inference
 * on a zero-filled input of the model's declared shape. Requires
 * `org.tensorflow:tensorflow-lite` in the app.
 */
class TfliteModelValidator : ModelValidator {
    override fun validate(file: File, model: ActiveModel): ValidationResult = try {
        org.tensorflow.lite.Interpreter(file).use { interpreter ->
            val inputs = Array<Any>(interpreter.inputTensorCount) { i ->
                val t = interpreter.getInputTensor(i)
                ByteBuffer.allocateDirect(t.numBytes()).order(ByteOrder.nativeOrder())
            }
            val outputs = HashMap<Int, Any>()
            for (i in 0 until interpreter.outputTensorCount) {
                val t = interpreter.getOutputTensor(i)
                outputs[i] = ByteBuffer.allocateDirect(t.numBytes()).order(ByteOrder.nativeOrder())
            }
            interpreter.runForMultipleInputsOutputs(inputs, outputs)
        }
        ValidationResult.Valid
    } catch (e: Throwable) {
        ValidationResult.Invalid("TFLite: ${e.message ?: e.javaClass.simpleName}")
    }
}

/**
 * Opens the model with ONNX Runtime and runs one inference on zero-filled
 * float inputs of the declared shapes (dynamic dimensions as 1). Requires
 * `com.microsoft.onnxruntime:onnxruntime-android` in the app.
 */
class OnnxModelValidator : ModelValidator {
    override fun validate(file: File, model: ActiveModel): ValidationResult = try {
        val env = ai.onnxruntime.OrtEnvironment.getEnvironment()
        env.createSession(file.absolutePath).use { session ->
            val tensors = session.inputInfo.mapValues { (_, info) ->
                val shape = (info.info as ai.onnxruntime.TensorInfo).shape.map { if (it < 1) 1L else it }.toLongArray()
                val count = shape.fold(1L) { a, b -> a * b }.toInt()
                ai.onnxruntime.OnnxTensor.createTensor(env, java.nio.FloatBuffer.allocate(count), shape)
            }
            try {
                session.run(tensors).close()
            } finally {
                tensors.values.forEach { it.close() }
            }
        }
        ValidationResult.Valid
    } catch (e: Throwable) {
        ValidationResult.Invalid("ONNX: ${e.message ?: e.javaClass.simpleName}")
    }
}

/** Picks a validator by file format and by which runtimes the app includes. */
object ModelValidators {
    /**
     * TFLite files (identifier "TFL3") are checked with [TfliteModelValidator]
     * if TensorFlow Lite is on the classpath; other files with
     * [OnnxModelValidator] if ONNX Runtime is; otherwise the model is accepted.
     */
    fun default(): ModelValidator = ModelValidator { file, model ->
        val validator = when {
            isTflite(file) && hasClass("org.tensorflow.lite.Interpreter") -> TfliteModelValidator()
            !isTflite(file) && hasClass("ai.onnxruntime.OrtEnvironment") -> OnnxModelValidator()
            else -> {
                Logger.d("No runtime to validate ${model.modelName} ${model.version}; accepting it")
                NoOpModelValidator
            }
        }
        validator.validate(file, model)
    }

    internal fun isTflite(file: File): Boolean = try {
        file.inputStream().use { input ->
            val header = ByteArray(8)
            input.read(header) == 8 && String(header, 4, 4, Charsets.US_ASCII) == "TFL3"
        }
    } catch (_: Exception) {
        false
    }

    private fun hasClass(name: String) = try {
        Class.forName(name)
        true
    } catch (_: Throwable) {
        false
    }
}
