package com.a2acli.transport

import io.ktor.client.*
import io.ktor.client.engine.mock.*
import io.ktor.client.plugins.contentnegotiation.*
import io.ktor.http.*
import io.ktor.serialization.kotlinx.json.*
import kotlinx.coroutines.flow.toList
import kotlinx.coroutines.test.runTest
import kotlinx.serialization.json.*
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

class HttpTransportTest {

    private val json = Json { ignoreUnknownKeys = true; isLenient = true }

    private fun mockClient(
        body: String,
        contentType: ContentType = ContentType.Application.Json,
        status: HttpStatusCode = HttpStatusCode.OK,
    ): HttpClient = HttpClient(MockEngine { request ->
        respond(body, status, headersOf(HttpHeaders.ContentType, contentType.toString()))
    }) {
        install(ContentNegotiation) { json(json) }
    }

    private fun jsonRpcResult(result: String) =
        """{"jsonrpc":"2.0","id":"1","result":$result}"""

    private fun jsonRpcError(code: Int, message: String) =
        """{"jsonrpc":"2.0","id":"1","error":{"code":$code,"message":"$message"}}"""

    // ── JSON response ─────────────────────────────────────────────────────────

    @Test
    fun `call returns result for JSON response`() = runTest {
        val resultJson = """{"id":"task-1","status":{"state":"completed"}}"""
        val transport = HttpTransport("http://localhost/rpc", testClient = mockClient(jsonRpcResult(resultJson)))
        val result = transport.call("tasks/send", buildJsonObject { put("id", "task-1") })
        assertEquals("task-1", result.jsonObject["id"]?.jsonPrimitive?.content)
    }

    @Test
    fun `call throws JsonRpcException for JSON-RPC error response`() = runTest {
        val transport = HttpTransport(
            "http://localhost/rpc",
            testClient = mockClient(jsonRpcError(-32001, "Task not found")),
        )
        val ex = runCatching {
            transport.call("tasks/get", buildJsonObject { put("id", "x") })
        }.exceptionOrNull()
        assertNotNull(ex)
        assertTrue(ex is JsonRpcException)
        assertEquals("Task not found", ex?.message)
        assertEquals(-32001, (ex as JsonRpcException).code)
    }

    // ── SSE response ──────────────────────────────────────────────────────────

    @Test
    fun `call returns first SSE event as result`() = runTest {
        val firstEvent = """{"jsonrpc":"2.0","id":"1","result":{"id":"t1","status":{"state":"working"}}}"""
        val secondEvent = """{"id":"t1","status":{"state":"completed"},"final":true}"""
        val sseBody = "data: $firstEvent\n\ndata: $secondEvent\n\n"
        val transport = HttpTransport(
            "http://localhost/rpc",
            testClient = mockClient(sseBody, ContentType("text", "event-stream")),
        )
        val result = transport.call("tasks/sendSubscribe", buildJsonObject { put("id", "t1") })
        assertEquals("t1", result.jsonObject["id"]?.jsonPrimitive?.content)
    }

    @Test
    fun `stream emits remaining SSE events after call`() = runTest {
        val firstEvent = """{"jsonrpc":"2.0","id":"1","result":{"id":"t1","status":{"state":"working"}}}"""
        val streamEvent = """{"id":"t1","status":{"state":"completed"},"final":true}"""
        val sseBody = "data: $firstEvent\n\ndata: $streamEvent\n\n"
        val transport = HttpTransport(
            "http://localhost/rpc",
            testClient = mockClient(sseBody, ContentType("text", "event-stream")),
        )
        transport.call("tasks/sendSubscribe", buildJsonObject { put("id", "t1") })
        val events = transport.stream().toList()
        assertEquals(1, events.size)
        assertEquals("completed", events[0]["status"]?.jsonObject?.get("state")?.jsonPrimitive?.content)
    }

    @Test
    fun `call throws JsonRpcException for SSE error in first event`() = runTest {
        val errorEvent = jsonRpcError(-32000, "Server error")
        val sseBody = "data: $errorEvent\n\n"
        val transport = HttpTransport(
            "http://localhost/rpc",
            testClient = mockClient(sseBody, ContentType("text", "event-stream")),
        )
        val ex = runCatching {
            transport.call("tasks/sendSubscribe", buildJsonObject { put("id", "t1") })
        }.exceptionOrNull()
        assertTrue(ex is JsonRpcException)
        assertEquals("Server error", ex?.message)
    }

    @Test
    fun `call throws for unsupported content type`() = runTest {
        val transport = HttpTransport(
            "http://localhost/rpc",
            testClient = mockClient("whatever", ContentType.Text.Plain),
        )
        val ex = runCatching {
            transport.call("tasks/send", buildJsonObject { put("id", "t1") })
        }.exceptionOrNull()
        assertTrue(ex is JsonRpcException)
        assertTrue(ex?.message?.contains("Unsupported") == true)
    }

    @Test
    fun `stream throws when called without a prior SSE response`() = runTest {
        val resultJson = """{"id":"t1","status":{"state":"completed"}}"""
        val transport = HttpTransport(
            "http://localhost/rpc",
            testClient = mockClient(jsonRpcResult(resultJson)),
        )
        transport.call("tasks/send", buildJsonObject { put("id", "t1") })
        val ex = runCatching { transport.stream().toList() }.exceptionOrNull()
        assertTrue(ex is IllegalStateException)
    }
}
