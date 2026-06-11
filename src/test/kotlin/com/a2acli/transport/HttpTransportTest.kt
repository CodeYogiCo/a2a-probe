package com.a2acli.transport

import com.sun.net.httpserver.HttpServer
import kotlinx.coroutines.flow.toList
import kotlinx.coroutines.runBlocking
import kotlinx.serialization.json.*
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test
import java.net.InetSocketAddress

class HttpTransportTest {

    private fun withServer(
        contentType: String,
        body: String,
        block: suspend (baseUrl: String) -> Unit,
    ) {
        val server = HttpServer.create(InetSocketAddress(0), 0)
        server.createContext("/rpc") { exchange ->
            val bytes = body.toByteArray(Charsets.UTF_8)
            exchange.responseHeaders["Content-Type"] = listOf(contentType)
            exchange.sendResponseHeaders(200, bytes.size.toLong())
            exchange.responseBody.use { it.write(bytes) }
        }
        server.start()
        val url = "http://localhost:${server.address.port}"
        try {
            runBlocking { block(url) }
        } finally {
            server.stop(0)
        }
    }

    private fun jsonRpcResult(result: String) =
        """{"jsonrpc":"2.0","id":"1","result":$result}"""

    private fun jsonRpcError(code: Int, message: String) =
        """{"jsonrpc":"2.0","id":"1","error":{"code":$code,"message":"$message"}}"""

    // ── JSON response ─────────────────────────────────────────────────────────

    @Test
    fun `call returns result for JSON response`() {
        val resultJson = """{"id":"task-1","status":{"state":"completed"}}"""
        withServer("application/json", jsonRpcResult(resultJson)) { url ->
            val transport = HttpTransport("$url/rpc")
            val result = transport.call("tasks/send", buildJsonObject { put("id", "task-1") })
            assertEquals("task-1", result.jsonObject["id"]?.jsonPrimitive?.content)
        }
    }

    @Test
    fun `call throws JsonRpcException for JSON-RPC error response`() {
        withServer("application/json", jsonRpcError(-32001, "Task not found")) { url ->
            val transport = HttpTransport("$url/rpc")
            val ex = runCatching {
                transport.call("tasks/get", buildJsonObject { put("id", "x") })
            }.exceptionOrNull()
            assertNotNull(ex)
            assertTrue(ex is JsonRpcException)
            assertEquals("Task not found", ex?.message)
            assertEquals(-32001, (ex as JsonRpcException).code)
        }
    }

    // ── SSE response ──────────────────────────────────────────────────────────

    @Test
    fun `call returns first SSE event as result`() {
        val firstEvent = jsonRpcResult("""{"id":"t1","status":{"state":"working"}}""")
        val secondEvent = """{"id":"t1","status":{"state":"completed"},"final":true}"""
        val sseBody = "data: $firstEvent\n\ndata: $secondEvent\n\n"
        withServer("text/event-stream", sseBody) { url ->
            val transport = HttpTransport("$url/rpc")
            val result = transport.call("tasks/sendSubscribe", buildJsonObject { put("id", "t1") })
            assertEquals("t1", result.jsonObject["id"]?.jsonPrimitive?.content)
        }
    }

    @Test
    fun `stream emits remaining SSE events after call`() {
        val firstEvent = jsonRpcResult("""{"id":"t1","status":{"state":"working"}}""")
        val streamEvent = """{"id":"t1","status":{"state":"completed"},"final":true}"""
        val sseBody = "data: $firstEvent\n\ndata: $streamEvent\n\n"
        withServer("text/event-stream", sseBody) { url ->
            val transport = HttpTransport("$url/rpc")
            transport.call("tasks/sendSubscribe", buildJsonObject { put("id", "t1") })
            val events = transport.stream().toList()
            assertEquals(1, events.size)
            assertEquals("completed", events[0]["status"]?.jsonObject?.get("state")?.jsonPrimitive?.content)
        }
    }

    @Test
    fun `call throws JsonRpcException for SSE error in first event`() {
        val sseBody = "data: ${jsonRpcError(-32000, "Server error")}\n\n"
        withServer("text/event-stream", sseBody) { url ->
            val transport = HttpTransport("$url/rpc")
            val ex = runCatching {
                transport.call("tasks/sendSubscribe", buildJsonObject { put("id", "t1") })
            }.exceptionOrNull()
            assertTrue(ex is JsonRpcException)
            assertEquals("Server error", ex?.message)
        }
    }

    @Test
    fun `call throws for unsupported content type`() {
        withServer("text/plain", "whatever") { url ->
            val transport = HttpTransport("$url/rpc")
            val ex = runCatching {
                transport.call("tasks/send", buildJsonObject { put("id", "t1") })
            }.exceptionOrNull()
            assertTrue(ex is JsonRpcException)
            assertTrue(ex?.message?.contains("Unsupported") == true)
        }
    }

    @Test
    fun `stream throws when called without a prior SSE response`() {
        val resultJson = """{"id":"t1","status":{"state":"completed"}}"""
        withServer("application/json", jsonRpcResult(resultJson)) { url ->
            val transport = HttpTransport("$url/rpc")
            transport.call("tasks/send", buildJsonObject { put("id", "t1") })
            val ex = runCatching { transport.stream().toList() }.exceptionOrNull()
            assertTrue(ex is IllegalStateException)
        }
    }
}
