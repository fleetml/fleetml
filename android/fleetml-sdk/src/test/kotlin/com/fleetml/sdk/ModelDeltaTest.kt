package com.fleetml.sdk

import com.fleetml.sdk.internal.ModelDelta
import org.junit.Assert.assertArrayEquals
import org.junit.Assert.assertThrows
import org.junit.Test
import java.io.File

/** The delta the server's Go code made: internal/delta/testdata. */
class ModelDeltaTest {
    private val dir = File("../../internal/delta/testdata")

    @Test
    fun `applies the server's delta`() {
        val out = ModelDelta.patch(File(dir, "old.bin").readBytes(), File(dir, "patch.fmldelta").readBytes())
        assertArrayEquals(File(dir, "new.bin").readBytes(), out)
    }

    @Test
    fun `rejects something that is not a delta`() {
        assertThrows(IllegalArgumentException::class.java) { ModelDelta.patch(ByteArray(10), "not a delta at all, sorry".toByteArray()) }
    }
}
