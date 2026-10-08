package com.fleetml.sdk

import kotlinx.coroutines.test.runTest
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import okhttp3.mockwebserver.SocketPolicy
import okio.Buffer
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import java.io.File
import java.security.MessageDigest
import kotlin.random.Random

class ModelDownloaderTest {
    private lateinit var server: MockWebServer
    private lateinit var dir: File
    private lateinit var cache: ModelCache
    private lateinit var downloader: ModelDownloader
    private val blob = Random(42).nextBytes(200_000)
    private val sha = sha256(blob)

    @Before
    fun setUp() {
        server = MockWebServer().apply { start() }
        dir = File(System.getProperty("java.io.tmpdir"), "fleetml-dl-${System.nanoTime()}")
        cache = ModelCache(dir, maxSizeMb = 10)
        downloader = ModelDownloader(cache)
    }

    @After
    fun tearDown() {
        server.shutdown()
        dir.deleteRecursively()
    }

    private val url get() = server.url("/models/v.bin").toString()

    @Test
    fun `streams the model, verifies it and renames it into place`() = runTest {
        server.enqueue(MockResponse().setBody(Buffer().write(blob)))
        val r = downloader.download("v", url, sha, blob.size.toLong())
        assertTrue(r is DownloadResult.Downloaded)
        assertTrue((r as DownloadResult.Downloaded).file.readBytes().contentEquals(blob))
        assertFalse(cache.partFile("v").exists())
    }

    @Test
    fun `an interrupted download resumes with a Range request`() = runTest {
        // First attempt: the connection drops halfway through the body.
        server.enqueue(MockResponse().setBody(Buffer().write(blob)).setSocketPolicy(SocketPolicy.DISCONNECT_DURING_RESPONSE_BODY))
        val first = downloader.download("v", url, sha, blob.size.toLong())
        assertTrue(first is DownloadResult.Failed)
        val have = cache.partFile("v").length()
        assertTrue("partial bytes are kept ($have)", have in 1 until blob.size)
        server.takeRequest()

        // Second attempt asks for the rest and gets 206.
        server.enqueue(MockResponse().setResponseCode(206).setBody(Buffer().write(blob.copyOfRange(have.toInt(), blob.size))))
        val second = downloader.download("v", url, sha, blob.size.toLong())
        assertEquals("bytes=$have-", server.takeRequest().getHeader("Range"))
        assertTrue(second is DownloadResult.Downloaded)
        assertTrue((second as DownloadResult.Downloaded).file.readBytes().contentEquals(blob))
    }

    @Test
    fun `a server that ignores Range sends the full file and the download starts over`() = runTest {
        cache.partFile("v").writeBytes(blob.copyOfRange(0, 1000))
        server.enqueue(MockResponse().setResponseCode(200).setBody(Buffer().write(blob)))
        val r = downloader.download("v", url, sha, blob.size.toLong())
        assertTrue(r is DownloadResult.Downloaded)
        assertEquals(blob.size.toLong(), (r as DownloadResult.Downloaded).file.length())
    }

    @Test
    fun `a corrupted download is rejected and nothing is kept`() = runTest {
        val corrupted = blob.copyOf().also { it[10] = (it[10] + 1).toByte() }
        server.enqueue(MockResponse().setBody(Buffer().write(corrupted)))
        val r = downloader.download("v", url, sha, blob.size.toLong())
        assertEquals(DownloadResult.Failed("SHA256 verification failed"), r)
        assertFalse(cache.partFile("v").exists())
        assertNull(cache.getModelFile("v"))
    }

    @Test
    fun `a cached model is not downloaded again`() = runTest {
        server.enqueue(MockResponse().setBody(Buffer().write(blob)))
        downloader.download("v", url, sha, blob.size.toLong())
        val r = downloader.download("v", url, sha, blob.size.toLong())
        assertTrue(r is DownloadResult.Cached)
        assertEquals(1, server.requestCount)
    }

    @Test
    fun `HTTP errors fail without touching the cache`() = runTest {
        server.enqueue(MockResponse().setResponseCode(403))
        assertEquals(DownloadResult.Failed("HTTP 403"), downloader.download("v", url, sha))
    }

    private fun sha256(data: ByteArray) = MessageDigest.getInstance("SHA-256").digest(data).joinToString("") { "%02x".format(it) }
}
