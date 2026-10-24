package com.fleetml.sample

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Test
import java.io.File
import javax.xml.parsers.DocumentBuilderFactory

/**
 * The debug build's cleartext exceptions: the emulator's and USB's addresses,
 * and the address given as -PfleetmlDebugHost (a phone over Wi-Fi: the LAN
 * address of the machine that runs the server, docs/ANDROID_SDK.md). The build
 * generates the file. Release builds have none.
 */
class NetworkSecurityConfigTest {
    private fun cleartextDomains(path: String): List<String> {
        val doc = DocumentBuilderFactory.newInstance().newDocumentBuilder().parse(File(path))
        val configs = doc.getElementsByTagName("domain-config")
        return (0 until configs.length).map { configs.item(it) as org.w3c.dom.Element }
            .filter { it.getAttribute("cleartextTrafficPermitted") == "true" }
            .flatMap { c -> c.getElementsByTagName("domain").let { d -> (0 until d.length).map { d.item(it).textContent.trim() } } }
    }

    @Test
    fun `debug allows the local addresses and the fleetmlDebugHost`() {
        val host = System.getProperty("fleetml.debugHost").orEmpty()
        assertEquals(
            (listOf("10.0.2.2", "localhost", "127.0.0.1") + host).filter { it.isNotEmpty() }.distinct(),
            cleartextDomains(System.getProperty("fleetml.networkConfig")),
        )
        assertFalse("no hand-edited copy is left", File("src/debug/res/xml/network_security_config.xml").exists())
    }

    @Test
    fun `release keeps Android's default`() {
        val manifest = File("src/main/AndroidManifest.xml").readText()
        assertFalse(manifest.contains("networkSecurityConfig"))
        assertFalse(manifest.contains("usesCleartextTraffic"))
        assertFalse(File("src/main/res/xml/network_security_config.xml").exists())
    }
}
