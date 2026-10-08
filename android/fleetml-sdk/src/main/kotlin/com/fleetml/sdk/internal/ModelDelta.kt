package com.fleetml.sdk.internal

import java.io.ByteArrayInputStream
import java.io.DataInputStream
import java.io.EOFException
import java.io.InputStream
import java.nio.ByteBuffer
import java.nio.ByteOrder
import java.util.zip.Inflater
import java.util.zip.InflaterInputStream

/**
 * Applies a model delta made by the server (internal/delta in the server
 * repository: bsdiff with raw DEFLATE sections). The cases in
 * internal/delta/testdata are tested here too.
 */
internal object ModelDelta {
    private const val MAGIC = "FMLDLT01"
    private const val HEADER = 8 + 3 * 8

    fun patch(old: ByteArray, patch: ByteArray): ByteArray {
        require(patch.size >= HEADER && String(patch, 0, 8, Charsets.US_ASCII) == MAGIC) { "not a FleetML delta" }
        val hdr = ByteBuffer.wrap(patch, 8, 24).order(ByteOrder.LITTLE_ENDIAN)
        val newSize = hdr.long
        val ctrlLen = hdr.long
        val diffLen = hdr.long
        require(newSize in 0..Int.MAX_VALUE && ctrlLen >= 0 && diffLen >= 0 && ctrlLen + diffLen <= patch.size - HEADER) { "corrupt delta header" }
        fun section(from: Long, to: Long): InputStream =
            InflaterInputStream(ByteArrayInputStream(patch, (HEADER + from).toInt(), (to - from).toInt()), Inflater(true))
        val ctrl = DataInputStream(section(0, ctrlLen))
        val diff = DataInputStream(section(ctrlLen, ctrlLen + diffLen))
        val extra = DataInputStream(section(ctrlLen + diffLen, patch.size.toLong() - HEADER))

        val out = ByteArray(newSize.toInt())
        var oldPos = 0L
        var newPos = 0
        val triple = ByteArray(24)
        while (newPos < newSize) {
            try {
                ctrl.readFully(triple)
            } catch (e: EOFException) {
                throw IllegalArgumentException("corrupt delta control")
            }
            val c = ByteBuffer.wrap(triple).order(ByteOrder.LITTLE_ENDIAN)
            val add = c.long
            val copy = c.long
            val seek = c.long
            require(add >= 0 && copy >= 0 && newPos + add + copy <= newSize) { "corrupt delta control" }
            diff.readFully(out, newPos, add.toInt())
            for (i in 0 until add.toInt()) {
                val p = oldPos + i
                if (p >= 0 && p < old.size) out[newPos + i] = (out[newPos + i] + old[p.toInt()]).toByte()
            }
            newPos += add.toInt()
            oldPos += add
            extra.readFully(out, newPos, copy.toInt())
            newPos += copy.toInt()
            oldPos += seek
        }
        return out
    }
}
