package com.a2acli.integration

import com.a2acli.A2AClient
import com.a2acli.model.*
import kotlinx.coroutines.flow.toList
import kotlinx.coroutines.test.runTest
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.put
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Tag
import org.junit.jupiter.api.Test
import org.junit.jupiter.api.Timeout
import java.util.UUID
import java.util.concurrent.TimeUnit
import kotlin.test.assertNotNull

/**
 * Functional tests against the live hello-world A2A agent.
 *
 * Skipped in the normal test run — execute with:
 *   ./gradlew test -Dintegration=true
 *
 * Note: the agent runs on Render's free tier and may take ~30 s to wake up.
 */
@Tag("integration")
@Timeout(value = 90, unit = TimeUnit.SECONDS)
class HelloWorldAgentTest {

    private val serverUrl = "https://hello-world-gxfr.onrender.com"
    private val client = A2AClient.overHttp(serverUrl)

    // ── agent card ────────────────────────────────────────────────────────────

    @Test
    fun `agent card is reachable and has required fields`() = runTest {
        val card = client.fetchAgentCard(serverUrl)
        assertNotNull(card, "Agent card should be reachable at /.well-known/agent.json")
        assertFalse(card!!.name.isNullOrBlank(), "Agent card must have a name")
        assertNotNull(card.url, "Agent card must have a url")
    }

    @Test
    fun `agent card url points to the server`() = runTest {
        val card = client.fetchAgentCard(serverUrl)
        assertNotNull(card?.url, "Agent card url must not be null")
        assertTrue(
            card!!.url!!.startsWith("http"),
            "Agent card url should be an http(s) URL, got: ${card.url}",
        )
    }

    @Test
    fun `agent card capabilities are present`() = runTest {
        val card = client.fetchAgentCard(serverUrl)
        assertNotNull(card?.capabilities, "Agent card should declare capabilities")
    }

    // ── send message ──────────────────────────────────────────────────────────

    @Test
    fun `send hello returns a response message`() = runTest {
        val response = client.sendMessage(helloMessage("Hello from integration test"))
        assertNotNull(response, "Response should not be null")
        assertTrue(
            response.role == Role.ASSISTANT || response.role == Role.AGENT,
            "Response role should be agent or assistant, got: ${response.role}",
        )
    }

    @Test
    fun `send hello response has non-empty parts`() = runTest {
        val response = client.sendMessage(helloMessage("Say hello"))
        assertTrue(response.parts.isNotEmpty(), "Response should have at least one part")
    }

    @Test
    fun `send hello response contains text`() = runTest {
        val response = client.sendMessage(helloMessage("Hello world"))
        val text = extractText(response.parts)
        assertFalse(text.isBlank(), "Response text should not be empty")
    }

    @Test
    fun `response includes a messageId`() = runTest {
        val response = client.sendMessage(helloMessage("Identify yourself"))
        assertFalse(response.messageId.isNullOrBlank(), "Response should include a messageId")
    }

    @Test
    fun `hello world agent responds with hello world text`() = runTest {
        val response = client.sendMessage(helloMessage("hi"))
        val text = extractText(response.parts).lowercase()
        assertTrue(
            text.contains("hello") || text.contains("world"),
            "Expected 'hello world' in response, got: $text",
        )
    }

    // ── streaming ─────────────────────────────────────────────────────────────

    @Test
    fun `streamMessage emits at least one event when agent supports streaming`() = runTest {
        val card = client.fetchAgentCard(serverUrl)
        if (card?.capabilities?.streaming != true) {
            println("Agent does not support streaming — skipping")
            return@runTest
        }
        val events = client.streamMessage(helloMessage("Stream hello")).toList()
        assertTrue(events.isNotEmpty(), "Streaming should emit at least one event")
    }

    // ── helpers ───────────────────────────────────────────────────────────────

    private fun helloMessage(text: String) = Message(
        role = Role.USER,
        parts = listOf(buildJsonObject { put("kind", "text"); put("text", text) }),
        messageId = UUID.randomUUID().toString(),
    )

    private fun extractText(parts: List<JsonElement>): String =
        parts.joinToString("") { part ->
            runCatching {
                part.jsonObject["text"]?.toString()?.trim('"') ?: ""
            }.getOrDefault("")
        }
}
