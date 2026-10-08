package com.fleetml.sdk

import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import java.io.File

class ModelValidatorTest {
    private lateinit var dir: File

    @Before
    fun setUp() {
        dir = File(System.getProperty("java.io.tmpdir"), "fleetml-val-${System.nanoTime()}").apply { mkdirs() }
    }

    @After
    fun tearDown() {
        dir.deleteRecursively()
    }

    private fun model(file: File) = ActiveModel("m", "iris", "1", "v", "x", "sha", file)

    @Test
    fun `TFLite files are recognised by their identifier`() {
        val tflite = File(dir, "a").apply { writeBytes(byteArrayOf(0x1c, 0, 0, 0, 'T'.code.toByte(), 'F'.code.toByte(), 'L'.code.toByte(), '3'.code.toByte(), 0)) }
        val other = File(dir, "b").apply { writeText("onnx protobuf bytes") }
        assertTrue(ModelValidators.isTflite(tflite))
        assertFalse(ModelValidators.isTflite(other))
        assertFalse(ModelValidators.isTflite(File(dir, "missing")))
    }

    @Test
    fun `without a runtime on the classpath the default validator accepts the model`() {
        // Unit tests run on the JVM without TFLite or ONNX Runtime.
        val f = File(dir, "m").apply { writeText("anything") }
        assertEquals(ValidationResult.Valid, ModelValidators.default().validate(f, model(f)))
        assertEquals(ValidationResult.Valid, NoOpModelValidator.validate(f, model(f)))
    }
}
