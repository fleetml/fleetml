package com.fleetml.sdk.internal

import fleetml.telemetry.v1.Telemetry.TelemetryEvent
import java.io.DataInputStream
import java.io.DataOutputStream
import java.io.EOFException
import java.io.File
import java.io.FileOutputStream

/**
 * A bounded FIFO of telemetry events persisted to [file], so events survive
 * process death and are kept while the device is offline or not yet
 * registered.
 *
 * Events are stored as length-prefixed protobuf records. Adding appends one
 * record; removing rewrites the file through a temp file and an atomic rename.
 * A truncated last record (e.g. power loss mid-write) is ignored on load.
 * When [maxEvents] is reached the oldest event is dropped and counted in [dropped].
 */
internal class TelemetryQueue(
    private val file: File,
    private val maxEvents: Int = DEFAULT_MAX_EVENTS,
) {
    private val lock = Any()
    private val events = ArrayDeque<TelemetryEvent>()

    /** Events dropped because the queue was full. */
    var dropped: Long = 0
        private set

    init {
        file.parentFile?.mkdirs()
        load()
    }

    val size: Int
        get() = synchronized(lock) { events.size }

    fun add(event: TelemetryEvent) = synchronized(lock) {
        events.addLast(event)
        if (events.size > maxEvents) {
            while (events.size > maxEvents) {
                events.removeFirst()
                dropped++
            }
            persist()
        } else {
            append(event)
        }
    }

    /** The oldest [n] events, without removing them. */
    fun peek(n: Int): List<TelemetryEvent> = synchronized(lock) { events.take(n) }

    /** Removes the oldest [n] events (after they were delivered). */
    fun remove(n: Int) = synchronized(lock) {
        repeat(minOf(n, events.size)) { events.removeFirst() }
        persist()
    }

    private fun append(event: TelemetryEvent) {
        DataOutputStream(FileOutputStream(file, true).buffered()).use { out ->
            val bytes = event.toByteArray()
            out.writeInt(bytes.size)
            out.write(bytes)
        }
    }

    private fun persist() {
        val tmp = File(file.parentFile, file.name + ".tmp")
        DataOutputStream(FileOutputStream(tmp).buffered()).use { out ->
            for (e in events) {
                val bytes = e.toByteArray()
                out.writeInt(bytes.size)
                out.write(bytes)
            }
        }
        if (!tmp.renameTo(file)) {
            file.delete()
            tmp.renameTo(file)
        }
    }

    private fun load() {
        if (!file.exists()) return
        try {
            DataInputStream(file.inputStream().buffered()).use { input ->
                while (true) {
                    val size = try {
                        input.readInt()
                    } catch (_: EOFException) {
                        break
                    }
                    if (size < 0 || size > MAX_RECORD_BYTES) break
                    val bytes = ByteArray(size)
                    try {
                        input.readFully(bytes)
                    } catch (_: EOFException) {
                        break // truncated last record
                    }
                    events.addLast(TelemetryEvent.parseFrom(bytes))
                }
            }
        } catch (e: Exception) {
            Logger.w("Telemetry queue unreadable, starting empty: ${e.message}")
            events.clear()
        }
        while (events.size > maxEvents) {
            events.removeFirst()
            dropped++
        }
        persist() // drop any truncated tail from the file
    }

    companion object {
        const val DEFAULT_MAX_EVENTS = 10_000
        private const val MAX_RECORD_BYTES = 64 * 1024
    }
}
