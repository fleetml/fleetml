package com.fleetml.sdk

import com.fleetml.sdk.internal.ModelRecord
import com.fleetml.sdk.internal.ModelStore
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import java.io.File

class ModelStoreTest {
    private lateinit var dir: File
    private val v1 = ModelRecord("m", "iris", "1.0.0", "ver1", "var1", "sha1")
    private val v2 = ModelRecord("m", "iris", "2.0.0", "ver2", "var2", "sha2")

    @Before
    fun setUp() {
        dir = File(System.getProperty("java.io.tmpdir"), "fleetml-store-${System.nanoTime()}")
    }

    @After
    fun tearDown() {
        dir.deleteRecursively()
    }

    private fun store() = ModelStore(File(dir, "models.json"))

    @Test
    fun `activating a new version keeps the previous one as last-known-good`() {
        val s = store()
        s.activate(v1)
        s.activate(v2)
        assertEquals(v2, s.active("m"))
        assertEquals(v1, s.lastKnownGood("m"))
        assertEquals(setOf("var1", "var2"), s.protectedVariants())
    }

    @Test
    fun `state survives a restart`() {
        store().apply { activate(v1); activate(v2); markFailed("var3", 1_000) }
        val reopened = store()
        assertEquals(v2, reopened.active("m"))
        assertEquals(v1, reopened.lastKnownGood("m"))
        assertTrue(reopened.recentlyFailed("var3", now = 2_000))
    }

    @Test
    fun `failed variants are retried after the cooldown`() {
        val s = store()
        s.markFailed("bad", 0)
        assertTrue(s.recentlyFailed("bad", now = ModelStore.FAILED_COOLDOWN_MS - 1))
        assertFalse(s.recentlyFailed("bad", now = ModelStore.FAILED_COOLDOWN_MS))
    }

    @Test
    fun `unload forgets the model`() {
        val s = store()
        s.activate(v1)
        s.unload("m")
        assertNull(s.active("m"))
        assertTrue(s.activeModels().isEmpty())
        assertFalse(s.isActive("var1"))
    }

    @Test
    fun `a corrupt store starts empty`() {
        File(dir, "models.json").apply { parentFile?.mkdirs(); writeText("{not json") }
        assertTrue(store().activeModels().isEmpty())
    }
}
