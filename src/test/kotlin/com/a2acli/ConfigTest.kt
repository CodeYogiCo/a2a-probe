package com.a2acli

import com.a2acli.model.ServerConfig
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.assertThrows
import org.junit.jupiter.api.io.TempDir
import java.io.File
import java.nio.file.Path

class ConfigTest {

    // ── resolveServerUrl ──────────────────────────────────────────────────────

    @Test
    fun `resolveServerUrl returns http URL as-is`() {
        assertEquals("http://localhost:8000", resolveServerUrl("http://localhost:8000"))
    }

    @Test
    fun `resolveServerUrl returns https URL as-is`() {
        assertEquals("https://example.com/agent", resolveServerUrl("https://example.com/agent"))
    }

    @Test
    fun `resolveServerUrl returns ws URL as-is`() {
        assertEquals("ws://localhost:9000", resolveServerUrl("ws://localhost:9000"))
    }

    @Test
    fun `resolveServerUrl returns wss URL as-is`() {
        assertEquals("wss://secure.example.com", resolveServerUrl("wss://secure.example.com"))
    }

    @Test
    fun `resolveServerUrl throws for unknown named server`() {
        assertThrows<IllegalStateException> {
            resolveServerUrl("nonexistent-server-name-xyz")
        }
    }

    // ── loadConfig / saveConfig ───────────────────────────────────────────────

    @Test
    fun `loadConfig returns empty config when file missing`(@TempDir tmp: Path) {
        withConfigDir(tmp) {
            val config = loadConfig()
            assertTrue(config.servers.isEmpty())
        }
    }

    @Test
    fun `saveConfig and loadConfig round-trip`(@TempDir tmp: Path) {
        withConfigDir(tmp) {
            val original = com.a2acli.model.CliConfig(
                servers = mapOf(
                    "local" to ServerConfig("http://localhost:8000", "http"),
                    "prod"  to ServerConfig("https://api.example.com", "sse"),
                )
            )
            saveConfig(original)
            val loaded = loadConfig()
            assertEquals(2, loaded.servers.size)
            assertEquals("http://localhost:8000", loaded.servers["local"]?.url)
            assertEquals("sse", loaded.servers["prod"]?.transport)
        }
    }

    @Test
    fun `loadConfig returns empty config on malformed JSON`(@TempDir tmp: Path) {
        withConfigDir(tmp) {
            val configFile = File(System.getProperty("user.home"), ".a2a/config.json")
            configFile.parentFile.mkdirs()
            configFile.writeText("{ broken json }")
            val config = loadConfig()
            assertTrue(config.servers.isEmpty())
        }
    }

    private fun withConfigDir(tmp: Path, block: () -> Unit) {
        val original = System.getProperty("user.home")
        System.setProperty("user.home", tmp.toAbsolutePath().toString())
        try {
            block()
        } finally {
            System.setProperty("user.home", original)
        }
    }
}
