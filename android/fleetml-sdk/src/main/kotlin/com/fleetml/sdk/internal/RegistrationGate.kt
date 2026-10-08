package com.fleetml.sdk.internal

import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock

/**
 * One registration at a time. Each Register call issues a new device token
 * that supersedes the previous one, so two concurrent registrations (start()
 * and syncNow() both finding the device unregistered) could leave the device
 * with the superseded token. A caller that waited skips the registration if
 * the other caller already did it.
 */
internal class RegistrationGate(private val needed: () -> Boolean) {
    private val mutex = Mutex()

    /** Runs [register] unless the device is registered by the time it is this caller's turn. */
    suspend fun registerIfNeeded(register: suspend () -> Unit) = mutex.withLock { if (needed()) register() }

    /** Runs [register] in turn, needed or not (the server rejected the current token). */
    suspend fun register(register: suspend () -> Unit) = mutex.withLock { register() }
}
