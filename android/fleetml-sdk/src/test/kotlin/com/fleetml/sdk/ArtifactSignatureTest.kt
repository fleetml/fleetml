package com.fleetml.sdk

import com.fleetml.sdk.internal.ArtifactSignature
import com.fleetml.sdk.internal.SigningKey
import org.json.JSONArray
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File
import java.util.Base64

/** The cases every client agrees on: internal/artifact/testdata/vectors.json. */
class ArtifactSignatureTest {
    private val vectors = File("../../internal/artifact/testdata/vectors.json")

    @Test
    fun `all clients agree on the signature vectors`() {
        assertTrue("run from android/fleetml-sdk: ${vectors.absolutePath}", vectors.exists())
        val cases = JSONArray(vectors.readText())
        assertTrue(cases.length() >= 10)
        val b64 = Base64.getDecoder()
        for (i in 0 until cases.length()) {
            val c = cases.getJSONObject(i)
            val keys = c.optJSONArray("keys")?.let { ks ->
                (0 until ks.length()).map { j ->
                    val k = ks.getJSONObject(j)
                    SigningKey(k.getString("key_id"), b64.decode(k.getString("public_key")), k.optLong("not_after"))
                }
            }.orEmpty()
            val sig = if (c.isNull("signature")) ByteArray(0) else b64.decode(c.getString("signature"))
            val reason = ArtifactSignature.verify(
                keys, c.getString("key_id"), sig, c.getString("model_id"), c.getString("version"), c.getString("sha256"), c.getLong("now"),
            )
            assertEquals("${c.getString("name")}: $reason", c.getBoolean("valid"), reason == null)
        }
    }
}
