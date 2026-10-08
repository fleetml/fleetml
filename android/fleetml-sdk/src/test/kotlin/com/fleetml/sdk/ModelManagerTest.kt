package com.fleetml.sdk

import com.fleetml.sdk.internal.ModelManager
import com.fleetml.sdk.internal.ModelStore
import com.fleetml.sdk.internal.TelemetryQueue
import fleetml.device.v1.Device.ModelAssignment
import fleetml.telemetry.v1.Telemetry.TelemetryEvent
import kotlinx.coroutines.test.runTest
import okhttp3.mockwebserver.Dispatcher
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import okhttp3.mockwebserver.RecordedRequest
import okio.Buffer
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import java.io.File
import java.security.MessageDigest

class ModelManagerTest {
    private lateinit var server: MockWebServer
    private lateinit var dir: File
    private lateinit var cache: ModelCache
    private lateinit var store: ModelStore
    private lateinit var queue: TelemetryQueue
    private val blobs = mapOf("good-v1" to "model one".toByteArray(), "good-v2" to "model two".toByteArray(), "broken" to "not a model".toByteArray())
    private val events = mutableListOf<String>()
    private var clock = 0L

    private val callback = object : FleetMLCallback {
        override fun onModelReady(model: ActiveModel) { events += "ready ${model.version}" }
        override fun onModelUnload(modelId: String, modelName: String, variantId: String) { events += "unload $variantId" }
        override fun onModelLoadFailed(modelName: String, version: String, reason: String) { events += "failed $version: $reason" }
    }

    /** Accepts every model except the one called "broken". */
    private val validator = ModelValidator { file, _ ->
        if (file.readText() == "not a model") ValidationResult.Invalid("cannot allocate tensors") else ValidationResult.Valid
    }

    @Before
    fun setUp() {
        server = MockWebServer()
        server.dispatcher = object : Dispatcher() {
            override fun dispatch(request: RecordedRequest): MockResponse =
                blobs[request.path!!.trimStart('/')]?.let { MockResponse().setBody(Buffer().write(it)) } ?: MockResponse().setResponseCode(404)
        }
        server.start()
        dir = File(System.getProperty("java.io.tmpdir"), "fleetml-mm-${System.nanoTime()}")
        cache = ModelCache(File(dir, "models"), maxSizeMb = 10)
        store = ModelStore(File(dir, "models.json"))
        queue = TelemetryQueue(File(dir, "q"))
    }

    @After
    fun tearDown() {
        server.shutdown()
        dir.deleteRecursively()
    }

    private fun manager(cacheOverride: ModelCache = cache): ModelManager {
        val reporter = TelemetryReporter(queue, { _, _, e -> e.size }, { null }, 50, 30, now = { clock })
        return ModelManager(store, cacheOverride, ModelDownloader(cacheOverride), validator, reporter, { callback }, now = { clock })
    }

    private fun assign(variant: String, version: String, action: ModelAssignment.Action = ModelAssignment.Action.LOAD) =
        ModelAssignment.newBuilder().setModelId("m").setModelName("iris").setVersion(version).setVersionId("ver-$version")
            .setVariantId(variant).setDownloadUrl(server.url("/$variant").toString())
            .setBlobSha256(blobs[variant]?.let(::sha256).orEmpty()).setBlobSizeBytes((blobs[variant]?.size ?: 0).toLong())
            .setAction(action).build()

    private fun unload(variant: String) = ModelAssignment.newBuilder().setModelId("m").setVariantId(variant).setAction(ModelAssignment.Action.UNLOAD).build()

    @Test
    fun `a valid model becomes active and is reported as loaded`() = runTest {
        val mm = manager()
        mm.apply(listOf(assign("good-v1", "1.0.0")))
        assertEquals(listOf("ready 1.0.0"), events)
        assertEquals("good-v1", mm.loadedStatuses().single().variantId)
        assertEquals("1.0.0", mm.activeModels().single().version)
        assertEquals(TelemetryEvent.EventType.MODEL_LOAD, queue.peek(10).single().type)
    }

    @Test
    fun `a model that fails validation never replaces the active one`() = runTest {
        val mm = manager()
        mm.apply(listOf(assign("good-v1", "1.0.0")))
        events.clear()

        // Server rolls out a broken v2: UPDATE(new) + UNLOAD(old) in one heartbeat.
        mm.apply(listOf(assign("broken", "2.0.0", ModelAssignment.Action.UPDATE), unload("good-v1")))

        assertEquals(listOf("failed 2.0.0: cannot allocate tensors"), events)
        assertEquals("the device keeps reporting v1", "good-v1", mm.loadedStatuses().single().variantId)
        assertNotNull("the last-known-good file is kept", cache.getModelFile("good-v1"))
        assertNull("the rejected file is deleted", cache.getModelFile("broken"))
        val failure = queue.peek(10).last()
        assertEquals(TelemetryEvent.EventType.MODEL_LOAD_FAILED, failure.type)
        assertEquals("cannot allocate tensors", failure.errorMessage)
        assertEquals("ver-2.0.0", failure.modelVersionId)

        // The next heartbeat repeats the assignment: not downloaded again within the cooldown.
        val before = server.requestCount
        mm.apply(listOf(assign("broken", "2.0.0", ModelAssignment.Action.UPDATE), unload("good-v1")))
        assertEquals(before, server.requestCount)
    }

    @Test
    fun `updating keeps the previous version as last-known-good on disk`() = runTest {
        val mm = manager()
        mm.apply(listOf(assign("good-v1", "1.0.0")))
        mm.apply(listOf(assign("good-v2", "2.0.0", ModelAssignment.Action.UPDATE), unload("good-v1")))
        assertEquals(listOf("ready 1.0.0", "ready 2.0.0"), events)
        assertEquals("good-v2", mm.loadedStatuses().single().variantId)
        assertEquals("good-v1", store.lastKnownGood("m")?.variantId)
        assertNotNull(cache.getModelFile("good-v1"))
    }

    @Test
    fun `eviction never removes the active or last-known-good model`() = runTest {
        val tiny = ModelCache(File(dir, "tiny"), maxSizeMb = 0)
        val mm = manager(tiny)
        mm.apply(listOf(assign("good-v1", "1.0.0")))
        mm.apply(listOf(assign("good-v2", "2.0.0", ModelAssignment.Action.UPDATE)))
        assertNotNull(tiny.getModelFile("good-v1"))
        assertNotNull(tiny.getModelFile("good-v2"))
    }

    @Test
    fun `downloads wait for the download policy`() = runTest {
        var unmetered = false
        val reporter = TelemetryReporter(queue, { _, _, e -> e.size }, { null }, 50, 30)
        val mm = ModelManager(store, cache, ModelDownloader(cache), validator, reporter, { callback }, canDownload = { urgent -> urgent || unmetered })
        mm.apply(listOf(assign("good-v1", "1.0.0")))
        assertEquals(0, server.requestCount)
        assertTrue(mm.loadedStatuses().isEmpty())
        unmetered = true
        mm.apply(listOf(assign("good-v1", "1.0.0")))
        assertEquals("ready 1.0.0", events.single())
    }

    @Test
    fun `an urgent assignment downloads on a metered network`() = runTest {
        val reporter = TelemetryReporter(queue, { _, _, e -> e.size }, { null }, 50, 30)
        val mm = ModelManager(store, cache, ModelDownloader(cache), validator, reporter, { callback },
            canDownload = { urgent -> BatteryPolicy.mayDownload(DownloadPolicy.UNMETERED_OR_CHARGING, urgent, connected = true, metered = true, charging = false) })
        mm.apply(listOf(assign("good-v1", "1.0.0")))
        assertEquals("not urgent: waits", 0, server.requestCount)
        mm.apply(listOf(assign("good-v1", "1.0.0").toBuilder().setUrgent(true).build()))
        assertEquals("ready 1.0.0", events.single())
    }

    @Test
    fun `a plain UNLOAD removes the model`() = runTest {
        val mm = manager()
        mm.apply(listOf(assign("good-v1", "1.0.0")))
        mm.apply(listOf(unload("good-v1")))
        assertTrue(mm.loadedStatuses().isEmpty())
        assertEquals("unload good-v1", events.last())
    }

    @Test
    fun `a failed download is retried on the next heartbeat`() = runTest {
        val mm = manager()
        val missing = assign("good-v1", "1.0.0").toBuilder().setDownloadUrl(server.url("/nope").toString()).build()
        mm.apply(listOf(missing))
        assertTrue(events.single().startsWith("failed 1.0.0: HTTP 404"))
        mm.apply(listOf(assign("good-v1", "1.0.0")))
        assertEquals("ready 1.0.0", events.last())
    }

    @Test
    fun `active models survive a restart (offline start)`() = runTest {
        manager().apply(listOf(assign("good-v1", "1.0.0")))
        val restarted = ModelManager(ModelStore(File(dir, "models.json")), cache, ModelDownloader(cache), validator, null, { null })
        assertEquals("good-v1", restarted.activeModels().single().variantId)
        assertEquals("model one", restarted.activeModels().single().file.readText())
    }

    private fun sha256(data: ByteArray) = MessageDigest.getInstance("SHA-256").digest(data).joinToString("") { "%02x".format(it) }

    @Test
    fun `a model whose signature does not verify is not downloaded or activated`() = runTest {
        val seed = ByteArray(32) { 7 }
        val priv = org.bouncycastle.crypto.params.Ed25519PrivateKeyParameters(seed, 0)
        val pub = priv.generatePublicKey().encoded
        store.setSigningKeys(listOf(com.fleetml.sdk.internal.SigningKey("k1", pub, 0)))
        fun signed(sha: String): ModelAssignment {
            val msg = com.fleetml.sdk.internal.ArtifactSignature.message("m", "1.0.0", sha)
            val sig = ByteArray(64).also { priv.sign(org.bouncycastle.math.ec.rfc8032.Ed25519.Algorithm.Ed25519, null, msg, 0, msg.size, it, 0) }
            return assign("good-v1", "1.0.0").toBuilder().setSigningKeyId("k1").setSignature(com.google.protobuf.ByteString.copyFrom(sig)).build()
        }
        val mm = manager()

        mm.apply(listOf(signed(sha256("another file".toByteArray()))))
        assertEquals("nothing is downloaded", 0, server.requestCount)
        assertTrue(mm.activeModels().isEmpty())
        assertTrue(events.single(), events.single().startsWith("failed 1.0.0: signature:"))
        assertEquals(TelemetryEvent.EventType.SIGNATURE_INVALID, queue.peek(10).last().type)

        clock += ModelStore.FAILED_COOLDOWN_MS
        mm.apply(listOf(signed(sha256(blobs.getValue("good-v1")))))
        assertEquals("1.0.0", mm.activeModels().single().version)
    }
}
