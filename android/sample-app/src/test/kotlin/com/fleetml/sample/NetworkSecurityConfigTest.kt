package com.fleetml.sample

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Test
import java.io.File
import javax.xml.parsers.DocumentBuilderFactory

/**
 * The debug build's cleartext exceptions: the emulator's and USB's addresses,
 * and one placeholder line that a phone over Wi-Fi replaces with the Mac's LAN
 * address (docs/ANDROID_SDK.md, "On your phone"). Release builds have none.
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
    fun `debug allows the local addresses and has the line for the LAN address`() {
        assertEquals(
            listOf("10.0.2.2", "localhost", "127.0.0.1", LAN_PLACEHOLDER),
            cleartextDomains("src/debug/res/xml/network_security_config.xml"),
        )
    }

    @Test
    fun `release keeps Android's default`() {
        val manifest = File("src/main/AndroidManifest.xml").readText()
        assertFalse(manifest.contains("networkSecurityConfig"))
        assertFalse(manifest.contains("usesCleartextTraffic"))
        assertFalse(File("src/main/res/xml/network_security_config.xml").exists())
    }

    companion object {
        const val LAN_PLACEHOLDER = "mac-lan-ip.invalid"
    }
}
