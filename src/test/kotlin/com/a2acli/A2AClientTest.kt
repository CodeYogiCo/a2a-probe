package com.a2acli

import com.a2acli.model.*
import com.a2acli.transport.JsonRpcException
import com.a2acli.transport.JsonRpcTransport
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.flowOf
import kotlinx.coroutines.flow.toList
import kotlinx.coroutines.test.runTest
import kotlinx.serialization.json.*
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

class A2AClientTest {

    // ── fake transport ────────────────────────────────────────────────────────

    private class FakeTransport : JsonRpcTransport {
        var lastMethod = ""
        var lastParams: JsonElement = JsonNull
        var nextResponse: JsonElement = JsonNull
        var nextException: Exception? = null
        val streamQueue = mutableListOf<JsonObject>()

        override suspend fun call(method: String, params: JsonElement): JsonElement {
            lastMethod = method
            lastParams = params
            nextException?.let { throw it }
            return nextResponse
        }
        override fun stream(): Flow<JsonObject> = flowOf(*streamQueue.toTypedArray())
        override suspend fun close() {}
    }

    // ── helpers ───────────────────────────────────────────────────────────────

    private fun taskJson(id: String = "task-1", state: String = "completed", text: String = "Done") =
        buildJsonObject {
            put("id", id)
            put("status", buildJsonObject {
                put("state", state)
                put("message", buildJsonObject {
                    put("kind", "message"); put("role", "assistant")
                    put("parts", buildJsonArray {
                        add(buildJsonObject { put("kind", "text"); put("text", text) })
                    })
                })
            })
        }

    private val sendParams = TaskSendParams(
        id = "task-1",
        message = Message(role = Role.USER, parts = listOf(
            buildJsonObject { put("kind", "text"); put("text", "Hello") }
        )),
    )

    // ── sendTask ──────────────────────────────────────────────────────────────

    @Test
    fun `sendTask calls tasks-send`() = runTest {
        val transport = FakeTransport().apply { nextResponse = taskJson() }
        A2AClient(transport).sendTask(sendParams)
        assertEquals("tasks/send", transport.lastMethod)
    }

    @Test
    fun `sendTask returns decoded Task`() = runTest {
        val transport = FakeTransport().apply { nextResponse = taskJson("t42", "completed", "Hi!") }
        val task = A2AClient(transport).sendTask(sendParams.copy(id = "t42"))
        assertEquals("t42", task.id)
        assertEquals(TaskState.COMPLETED, task.status.state)
    }

    @Test
    fun `sendTask encodes message kind in params`() = runTest {
        val transport = FakeTransport().apply { nextResponse = taskJson() }
        A2AClient(transport).sendTask(sendParams)
        val msgKind = transport.lastParams.jsonObject["message"]
            ?.jsonObject?.get("kind")?.jsonPrimitive?.content
        assertEquals("message", msgKind)
    }

    @Test
    fun `sendTask propagates JsonRpcException`() = runTest {
        val transport = FakeTransport().apply {
            nextException = JsonRpcException("Not found", -32001)
        }
        val ex = runCatching { A2AClient(transport).sendTask(sendParams) }.exceptionOrNull()
        assertTrue(ex is JsonRpcException)
        assertEquals("Not found", ex?.message)
        assertEquals(-32001, (ex as JsonRpcException).code)
    }

    // ── getTask ───────────────────────────────────────────────────────────────

    @Test
    fun `getTask calls tasks-get`() = runTest {
        val transport = FakeTransport().apply { nextResponse = taskJson() }
        A2AClient(transport).getTask(TaskQueryParams(id = "task-1"))
        assertEquals("tasks/get", transport.lastMethod)
    }

    @Test
    fun `getTask returns decoded Task`() = runTest {
        val transport = FakeTransport().apply { nextResponse = taskJson("t99", "failed") }
        val task = A2AClient(transport).getTask(TaskQueryParams(id = "t99"))
        assertEquals("t99", task.id)
        assertEquals(TaskState.FAILED, task.status.state)
    }

    @Test
    fun `getTask passes historyLength in params`() = runTest {
        val transport = FakeTransport().apply { nextResponse = taskJson() }
        A2AClient(transport).getTask(TaskQueryParams(id = "t1", historyLength = 5))
        assertEquals(5, transport.lastParams.jsonObject["historyLength"]?.jsonPrimitive?.int)
    }

    // ── cancelTask ────────────────────────────────────────────────────────────

    @Test
    fun `cancelTask calls tasks-cancel`() = runTest {
        val transport = FakeTransport().apply { nextResponse = JsonNull }
        A2AClient(transport).cancelTask(TaskIdParams(id = "task-1"))
        assertEquals("tasks/cancel", transport.lastMethod)
    }

    @Test
    fun `cancelTask passes correct task id`() = runTest {
        val transport = FakeTransport().apply { nextResponse = JsonNull }
        A2AClient(transport).cancelTask(TaskIdParams(id = "my-task"))
        assertEquals("my-task", transport.lastParams.jsonObject["id"]?.jsonPrimitive?.content)
    }

    // ── sendSubscribe / streaming ─────────────────────────────────────────────

    @Test
    fun `sendSubscribe calls tasks-sendSubscribe`() = runTest {
        val transport = FakeTransport()
        A2AClient(transport).sendSubscribe(sendParams).toList()
        assertEquals("tasks/sendSubscribe", transport.lastMethod)
    }

    @Test
    fun `sendSubscribe emits status events`() = runTest {
        val transport = FakeTransport().apply {
            streamQueue += buildJsonObject {
                put("id", "t1"); put("final", false)
                put("status", buildJsonObject { put("state", "working") })
            }
            streamQueue += buildJsonObject {
                put("id", "t1"); put("final", true)
                put("status", buildJsonObject { put("state", "completed") })
            }
        }
        val events = A2AClient(transport).sendSubscribe(sendParams).toList()
        assertEquals(2, events.size)
        assertFalse((events[0] as StreamEvent.Status).event.final)
        assertTrue((events[1] as StreamEvent.Status).event.final)
        assertEquals(TaskState.COMPLETED, (events[1] as StreamEvent.Status).event.status.state)
    }

    @Test
    fun `sendSubscribe emits artifact events`() = runTest {
        val transport = FakeTransport().apply {
            streamQueue += buildJsonObject {
                put("id", "t1")
                put("artifact", buildJsonObject {
                    put("index", 0)
                    put("parts", buildJsonArray {
                        add(buildJsonObject { put("kind", "text"); put("text", "chunk") })
                    })
                })
            }
        }
        val events = A2AClient(transport).sendSubscribe(sendParams).toList()
        assertEquals(1, events.size)
        assertEquals(0, (events[0] as StreamEvent.Artifact).event.artifact.index)
    }

    @Test
    fun `sendSubscribe emits Unknown for unrecognised events`() = runTest {
        val transport = FakeTransport().apply {
            streamQueue += buildJsonObject { put("weirdField", "value") }
        }
        val events = A2AClient(transport).sendSubscribe(sendParams).toList()
        assertTrue(events[0] is StreamEvent.Unknown)
    }

    @Test
    fun `sendSubscribe unwraps tasks-event envelope`() = runTest {
        val transport = FakeTransport().apply {
            streamQueue += buildJsonObject {
                put("method", "tasks/event")
                put("params", buildJsonObject {
                    put("id", "t1"); put("final", true)
                    put("status", buildJsonObject { put("state", "completed") })
                })
            }
        }
        val events = A2AClient(transport).sendSubscribe(sendParams).toList()
        assertTrue(events[0] is StreamEvent.Status)
    }

    // ── resubscribe ───────────────────────────────────────────────────────────

    @Test
    fun `resubscribe calls tasks-resubscribe`() = runTest {
        val transport = FakeTransport()
        A2AClient(transport).resubscribe(TaskQueryParams(id = "t1")).toList()
        assertEquals("tasks/resubscribe", transport.lastMethod)
    }
}
