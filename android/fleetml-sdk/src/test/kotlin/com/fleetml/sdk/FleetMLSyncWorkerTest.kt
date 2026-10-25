package com.fleetml.sdk

import io.grpc.Status
import io.grpc.StatusException
import org.junit.Assert.assertEquals
import org.junit.Test
import java.io.IOException

class FleetMLSyncWorkerTest {
    @Test
    fun `a refused registration is not an unreachable server`() {
        val refused = StatusException(Status.UNAUTHENTICATED.withDescription("invalid or revoked enrollment key"))
        assertEquals(
            "server reached, request refused (UNAUTHENTICATED: invalid or revoked enrollment key)",
            describeSync(false, refused),
        )
    }

    @Test
    fun `no answer is an unreachable server`() {
        assertEquals("server unreachable", describeSync(false, StatusException(Status.UNAVAILABLE.withDescription("Unable to resolve host"))))
        assertEquals("server unreachable", describeSync(false, StatusException(Status.DEADLINE_EXCEEDED)))
        assertEquals("server unreachable", describeSync(false, IOException("connection refused")))
        assertEquals("server unreachable", describeSync(false, null))
    }

    @Test
    fun `a sync that reached the server`() {
        assertEquals("server reached", describeSync(true, null))
    }
}
