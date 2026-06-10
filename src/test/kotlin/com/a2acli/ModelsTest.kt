package com.a2acli

import com.a2acli.model.*
import kotlinx.serialization.encodeToString
import kotlinx.serialization.json.*
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

class ModelsTest {

    private val json = Json {
        ignoreUnknownKeys = true
        isLenient = true
        encodeDefaults = false
        explicitNulls = false
    }

    // ── Message ───────────────────────────────────────────────────────────────

    @Test
    fun `Message always serializes kind as message`() {
        val msg = Message(
            role = Role.USER,
            parts = listOf(buildJsonObject { put("kind", "text"); put("text", "hi") }),
            messageId = "m1",
        )
        val obj = Json.parseToJsonElement(json.encodeToString(msg)).jsonObject
        assertEquals("message", obj["kind"]?.jsonPrimitive?.content)
    }

    @Test
    fun `Message role serializes as lowercase string`() {
        val user = Json.parseToJsonElement(json.encodeToString(Message(role = Role.USER, parts = emptyList()))).jsonObject
        val agent = Json.parseToJsonElement(json.encodeToString(Message(role = Role.ASSISTANT, parts = emptyList()))).jsonObject
        assertEquals("user", user["role"]?.jsonPrimitive?.content)
        assertEquals("assistant", agent["role"]?.jsonPrimitive?.content)
    }

    @Test
    fun `Message with unknown fields round-trips`() {
        val raw = """{"kind":"message","role":"user","parts":[],"unknownField":"ignored"}"""
        val msg = json.decodeFromString<Message>(raw)
        assertEquals(Role.USER, msg.role)
    }

    // ── Parts ─────────────────────────────────────────────────────────────────

    @Test
    fun `text part uses kind not type`() {
        val part = buildJsonObject { put("kind", "text"); put("text", "Hello") }
        assertEquals("text", part["kind"]?.jsonPrimitive?.content)
        assertNull(part["type"])
    }

    @Test
    fun `TaskSendParams embeds part kind in serialized output`() {
        val params = TaskSendParams(
            id = "t1",
            message = Message(
                role = Role.USER,
                parts = listOf(buildJsonObject { put("kind", "text"); put("text", "Hello") }),
            ),
        )
        val obj = Json.parseToJsonElement(json.encodeToString(params)).jsonObject
        val partObj = obj["message"]?.jsonObject
            ?.get("parts")?.jsonArray
            ?.get(0)?.jsonObject
        assertEquals("text", partObj?.get("kind")?.jsonPrimitive?.content)
        assertNull(partObj?.get("type"))
    }

    // ── Task ──────────────────────────────────────────────────────────────────

    @Test
    fun `Task deserializes with completed state and message`() {
        val raw = """
        {
          "id": "task-1",
          "sessionId": "sess-1",
          "status": {
            "state": "completed",
            "message": {
              "kind": "message",
              "role": "assistant",
              "parts": [{"kind": "text", "text": "Hello!"}]
            }
          }
        }
        """.trimIndent()
        val task = json.decodeFromString<Task>(raw)
        assertEquals("task-1", task.id)
        assertEquals(TaskState.COMPLETED, task.status.state)
        assertEquals(Role.ASSISTANT, task.status.message?.role)
    }

    @Test
    fun `Task deserializes all task states`() {
        val states = mapOf(
            "submitted" to TaskState.SUBMITTED,
            "working"   to TaskState.WORKING,
            "completed" to TaskState.COMPLETED,
            "failed"    to TaskState.FAILED,
            "canceled"  to TaskState.CANCELED,
            "unknown"   to TaskState.UNKNOWN,
        )
        for ((wire, expected) in states) {
            val task = json.decodeFromString<Task>("""{"id":"t","status":{"state":"$wire"}}""")
            assertEquals(expected, task.status.state, "state=$wire")
        }
    }

    @Test
    fun `Task deserializes artifacts`() {
        val raw = """
        {
          "id": "t2",
          "status": {"state": "completed"},
          "artifacts": [
            {"name": "out", "parts": [{"kind": "text", "text": "World"}], "index": 0}
          ]
        }
        """.trimIndent()
        val task = json.decodeFromString<Task>(raw)
        assertEquals(1, task.artifacts?.size)
        assertEquals("out", task.artifacts?.first()?.name)
    }

    // ── AgentCard ─────────────────────────────────────────────────────────────

    @Test
    fun `AgentCard deserializes capabilities`() {
        val raw = """
        {
          "name": "Test Agent",
          "url": "https://example.com",
          "version": "1.0",
          "capabilities": {"streaming": true, "pushNotifications": false}
        }
        """.trimIndent()
        val card = json.decodeFromString<AgentCard>(raw)
        assertEquals("Test Agent", card.name)
        assertTrue(card.capabilities?.streaming == true)
        assertFalse(card.capabilities?.pushNotifications == true)
    }

    @Test
    fun `AgentCard tolerates missing optional fields`() {
        val card = json.decodeFromString<AgentCard>("""{"name":"Minimal"}""")
        assertEquals("Minimal", card.name)
        assertNull(card.capabilities)
        assertNull(card.skills)
    }

    // ── Streaming events ──────────────────────────────────────────────────────

    @Test
    fun `TaskStatusUpdateEvent deserializes`() {
        val raw = """{"id":"t1","status":{"state":"working"},"final":false}"""
        val event = json.decodeFromString<TaskStatusUpdateEvent>(raw)
        assertEquals("t1", event.id)
        assertEquals(TaskState.WORKING, event.status.state)
        assertFalse(event.final)
    }

    @Test
    fun `TaskArtifactUpdateEvent deserializes`() {
        val raw = """
        {
          "id": "t1",
          "artifact": {
            "index": 0,
            "parts": [{"kind": "text", "text": "chunk"}]
          }
        }
        """.trimIndent()
        val event = json.decodeFromString<TaskArtifactUpdateEvent>(raw)
        assertEquals("t1", event.id)
        assertEquals(0, event.artifact.index)
    }
}
