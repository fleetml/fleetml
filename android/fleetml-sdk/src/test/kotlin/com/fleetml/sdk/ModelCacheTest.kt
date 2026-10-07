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

    private lateinit var cacheDir: File
    private lateinit var cache: ModelCache

    @Before
    fun setUp() {
        cacheDir = File(System.getProperty("java.io.tmpdir"), "fleetml-test-cache-${System.nanoTime()}")
        cacheDir.mkdirs()
        cache = ModelCache(cacheDir, maxSizeMb = 10)
    }

    @After
    fun tearDown() {
        cacheDir.deleteRecursively()
    }

    @Test
    fun `save and retrieve model`() {
        val data = "model data".toByteArray()
        val sha = sha256(data)

        val file = cache.save("variant-1", data, sha)
        assertNotNull(file)
        assertTrue(file!!.exists())
        assertEquals("model data", file.readText())
    }

    @Test
    fun `isCached returns true for valid cached model`() {
        val data = "cached model".toByteArray()
        val sha = sha256(data)

        cache.save("variant-2", data, sha)
        assertTrue(cache.isCached("variant-2", sha))
    }

    @Test
    fun `isCached returns false for wrong SHA256`() {
        val data = "cached model".toByteArray()
        val sha = sha256(data)

        cache.save("variant-3", data, sha)
        assertFalse(cache.isCached("variant-3", "wrong-sha"))
    }

    @Test
    fun `isCached returns false for missing file`() {
        assertFalse(cache.isCached("nonexistent", "any-sha"))
    }

    @Test
    fun `save fails on SHA256 mismatch`() {
        val data = "model data".toByteArray()
        val file = cache.save("variant-4", data, "wrong-sha256")
        assertNull(file)
    }

    @Test
    fun `getModelFile returns null for uncached variant`() {
        assertNull(cache.getModelFile("missing"))
    }

    @Test
    fun `getModelFile returns file for cached variant`() {
        val data = "test".toByteArray()
        cache.save("variant-5", data, sha256(data))
        assertNotNull(cache.getModelFile("variant-5"))
    }

    @Test
    fun `eviction removes oldest files when over limit`() {
        val smallCache = ModelCache(cacheDir, maxSizeMb = 0) // 0MB limit forces eviction

        val data1 = ByteArray(100) { 1 }
        val data2 = ByteArray(100) { 2 }

        smallCache.save("old", data1, sha256(data1))
        Thread.sleep(10) // Ensure different last modified
        smallCache.save("new", data2, sha256(data2))

        // Old file should be evicted (cache limit is 0)
        assertNull(smallCache.getModelFile("old"))
    }

    @Test
    fun `clear removes all files`() {
        val data = "data".toByteArray()
        cache.save("v1", data, sha256(data))
        cache.save("v2", data, sha256(data))

        cache.clear()
        assertEquals(0L, cache.currentSizeBytes())
    }

    @Test
    fun `currentSizeBytes reflects cache contents`() {
        assertEquals(0L, cache.currentSizeBytes())

        val data = ByteArray(1024)
        cache.save("variant", data, sha256(data))

        assertTrue(cache.currentSizeBytes() > 0)
    }

    private fun sha256(data: ByteArray): String {
        val digest = MessageDigest.getInstance("SHA-256")
        return digest.digest(data).joinToString("") { "%02x".format(it) }
    }
}
