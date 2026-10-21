package com.fleetml.sdk

import com.fleetml.sdk.internal.Logger
import com.fleetml.sdk.internal.ModelManager
import com.fleetml.sdk.internal.ModelStore
import com.fleetml.sdk.internal.TelemetryQueue
import com.fleetml.sdk.internal.TokenStore
import fleetml.device.v1.Device.ExperimentAssignment
import fleetml.device.v1.Device.HeartbeatRequest
import fleetml.device.v1.Device.HeartbeatResponse
import fleetml.device.v1.Device.RefreshTokenResponse
import io.grpc.Status
import io.grpc.StatusException
import kotlinx.coroutines.test.runTest
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import java.io.File

class HeartbeatManagerTest {
    private lateinit var dir: File
    private val day = 24L * 60 * 60 * 1000
    private val now = 1_800_000_000_000L

    private class MemTokens(var token: String?, var expiresAt: Long) : TokenStore {
        override fun getDeviceId() = "dev-1"
        override fun getAuthToken() = token
        override fun getTokenExpiresAtMillis() = expiresAt
        override fun saveToken(authToken: String, tokenExpiresAtSeconds: Long) {
            token = authToken
            expiresAt = tokenExpiresAtSeconds * 1000
        }
    }

    private class FakeApi : DeviceApi {
        val requests = mutableListOf<Pair<String, HeartbeatRequest>>()
        var refreshes = 0
        var failWith: Status? = null
        var response: HeartbeatResponse = HeartbeatResponse.getDefaultInstance()
        override suspend fun heartbeat(token: String, request: HeartbeatRequest): HeartbeatResponse {
            failWith?.let { throw StatusException(it) }
            requests += token to request
            return response
        }
        override suspend fun refreshToken(token: String): RefreshTokenResponse {
            refreshes++
            return RefreshTokenResponse.newBuilder().setAuthToken("refreshed").setTokenExpiresAt(4_000_000_000L).build()
        }
    }

    @Before
    fun setUp() {
        dir = File(System.getProperty("java.io.tmpdir"), "fleetml-hb-${System.nanoTime()}")
    }

    @After
    fun tearDown() {
        dir.deleteRecursively()
    }

    private val sent = mutableListOf<Int>()
    private val connection = mutableListOf<Boolean>()

    private val configs = mutableListOf<fleetml.device.v1.Device.DeviceConfig>()

    private fun setup(tokens: MemTokens, api: FakeApi, reRegister: suspend () -> Unit = {}): Pair<HeartbeatManager, TelemetryReporter> {
        val cache = ModelCache(File(dir, "models"), 10)
        val reporter = TelemetryReporter(TelemetryQueue(File(dir, "q")), { _, _, e -> sent += e.size; e.size },
            { tokens.token?.let { DeviceCredentials("dev-1", it) } }, 50, 30)
        val models = ModelManager(ModelStore(File(dir, "models.json")), cache, ModelDownloader(cache), NoOpModelValidator, reporter, { null })
        val cb = object : FleetMLCallback {
            override fun onConnectionStatusChanged(connected: Boolean) { connection += connected }
        }
        return HeartbeatManager(api, tokens, models, reporter, { cb }, 30, reRegister, onConfig = { configs += it }, now = { now }) to reporter
    }

    @Test
    fun `applies the server interval and experiment arms`() = runTest {
        val api = FakeApi().apply {
            response = HeartbeatResponse.newBuilder().setNextHeartbeatSeconds(90)
                .addExperiments(ExperimentAssignment.newBuilder().setModelVersionId("v-arm").setArmId("arm-7")).build()
        }
        val (hb, reporter) = setup(MemTokens("tok", now + 30 * day), api)
        assertTrue(hb.beatOnce())
        assertEquals(90L, hb.intervalSeconds)
        assertEquals("dev-1", api.requests.single().second.deviceId)
        reporter.reportInference("v-arm", 5)
        reporter.flushNow()
        assertEquals(1, sent.single())
    }

    @Test
    fun `passes the heartbeat's device config on`() = runTest {
        val config = fleetml.device.v1.Device.DeviceConfig.newBuilder()
            .setDownloadPolicy(fleetml.device.v1.Device.DownloadPolicy.ANY_NETWORK).setBackgroundHeartbeatIntervalSeconds(7200).build()
        val api = FakeApi().apply { response = HeartbeatResponse.newBuilder().setNextHeartbeatSeconds(60).setConfig(config).build() }
        val (hb, _) = setup(MemTokens("tok", now + 30 * day), api)
        hb.beatOnce()
        assertEquals(listOf(config), configs)
    }

    @Test
    fun `refreshes the token in its last days and uses the new one`() = runTest {
        val tokens = MemTokens("old", now + 2 * day)
        val api = FakeApi()
        val (hb, _) = setup(tokens, api)
        hb.beatOnce()
        assertEquals(1, api.refreshes)
        assertEquals("refreshed", api.requests.single().first)
        assertEquals("refreshed", tokens.token)

        hb.beatOnce()
        assertEquals("a fresh token is not refreshed again", 1, api.refreshes)
    }

    @Test
    fun `a rejected token triggers re-registration`() = runTest {
        var reRegistered = 0
        val api = FakeApi().apply { failWith = Status.UNAUTHENTICATED }
        val (hb, _) = setup(MemTokens("revoked", now + 30 * day), api) { reRegistered++ }
        assertFalse(hb.beatOnce())
        assertEquals(1, reRegistered)
    }

    @Test
    fun `connection changes are reported once and reconnecting flushes telemetry`() = runTest {
        val api = FakeApi().apply { failWith = Status.UNAVAILABLE }
        val (hb, _) = setup(MemTokens("tok", now + 30 * day), api)
        hb.beatOnce()
        hb.beatOnce()
        api.failWith = null
        hb.beatOnce()
        assertEquals(listOf(false, true), connection)
    }

    @Test
    fun `an unregistered device does not heartbeat`() = runTest {
        val api = FakeApi()
        val (hb, _) = setup(MemTokens(null, 0), api)
        assertFalse(hb.beatOnce())
        assertTrue(api.requests.isEmpty())
    }

    @Test
    fun `an offline heartbeat logs one warning line and keeps the stack trace at debug`() = runTest {
        val lines = mutableListOf<Triple<Int, String, Throwable?>>()
        val saved = Logger.sink
        Logger.sink = { p, m, t -> lines += Triple(p, m, t) }
        try {
            val cause = java.net.ConnectException("failed to connect to /10.0.2.2 (port 19090) from /:: (port 0) after 20000ms: connect failed: ENETUNREACH (Network is unreachable)")
            val api = FakeApi().apply { failWith = Status.UNAVAILABLE.withDescription("io exception").withCause(cause) }
            val (hb, _) = setup(MemTokens("tok", now + 30 * day), api)
            assertFalse(hb.beatOnce())
        } finally {
            Logger.sink = saved
        }
        val warnings = lines.filter { it.first >= android.util.Log.WARN }
        assertEquals(
            listOf("Heartbeat failed: UNAVAILABLE: io exception: failed to connect to /10.0.2.2 (port 19090) from /:: (port 0) after 20000ms: connect failed: ENETUNREACH (Network is unreachable)"),
            warnings.map { it.second },
        )
        assertTrue("no stack trace above debug", warnings.all { it.third == null })
        assertTrue("the stack trace is at debug", lines.any { it.first == android.util.Log.DEBUG && it.third != null })
    }

    @Test
    fun `describe keeps a plain exception's message`() {
        assertEquals("boom", Logger.describe(IllegalStateException("boom")))
        assertEquals("DEADLINE_EXCEEDED", Logger.describe(StatusException(Status.DEADLINE_EXCEEDED)))
    }
}
