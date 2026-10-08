package com.fleetml.sdk

import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import java.io.File
import java.security.MessageDigest

class ModelCacheTest {
    private lateinit var dir: File
    private lateinit var cache: ModelCache

    @Before
    fun setUp() {
        dir = File(System.getProperty("java.io.tmpdir"), "fleetml-cache-${System.nanoTime()}")
        cache = ModelCache(dir, maxSizeMb = 10)
    }

    @After
    fun tearDown() {
        dir.deleteRecursively()
    }

    private fun put(c: ModelCache, variant: String, bytes: ByteArray): File? {
        c.partFile(variant).writeBytes(bytes)
        return c.commit(variant, c.partFile(variant), sha256(bytes))
    }

    @Test
    fun `commit verifies and stores the model`() {
        val file = put(cache, "v1", "model data".toByteArray())
        assertNotNull(file)
        assertEquals("model data", file!!.readText())
        assertTrue(cache.isCached("v1", sha256("model data".toByteArray())))
        assertFalse("the part is renamed into place", cache.partFile("v1").exists())
    }

    @Test
    fun `commit rejects a wrong sha256 and deletes the part`() {
        cache.partFile("v2").writeBytes("tampered".toByteArray())
        assertNull(cache.commit("v2", cache.partFile("v2"), sha256("original".toByteArray())))
        assertFalse(cache.partFile("v2").exists())
        assertNull(cache.getModelFile("v2"))
    }

    @Test
    fun `isCached compares the stored hash`() {
        put(cache, "v3", "x".toByteArray())
        assertFalse(cache.isCached("v3", "other"))
        assertFalse(cache.isCached("missing", "any"))
    }

    @Test
    fun `eviction never removes protected models`() {
        val tiny = ModelCache(dir, maxSizeMb = 0) // every insert forces eviction
        put(tiny, "active", ByteArray(100) { 1 })
        put(tiny, "lkg", ByteArray(100) { 2 })
        put(tiny, "old", ByteArray(100) { 3 })
        File(dir, "old.bin").setLastModified(1) // least recently used
        tiny.evictIfNeeded(100, protected = setOf("active", "lkg"))
        assertNotNull(tiny.getModelFile("active"))
        assertNotNull(tiny.getModelFile("lkg"))
        assertNull(tiny.getModelFile("old"))
    }

    @Test
    fun `cleanup removes temp files and stale parts but keeps fresh parts`() {
        File(dir, "x.tmp").writeText("t")
        val stale = cache.partFile("stale").apply { writeText("p"); setLastModified(1_000) }
        val fresh = cache.partFile("fresh").apply { writeText("p") }
        cache.cleanupTemp(maxPartAgeMs = 60_000, now = System.currentTimeMillis())
        assertFalse(File(dir, "x.tmp").exists())
        assertFalse(stale.exists())
        assertTrue(fresh.exists())
    }

    @Test
    fun `clear empties the cache`() {
        put(cache, "a", "1".toByteArray())
        cache.clear()
        assertEquals(0L, cache.currentSizeBytes())
    }

    private fun sha256(data: ByteArray) = MessageDigest.getInstance("SHA-256").digest(data).joinToString("") { "%02x".format(it) }
}
