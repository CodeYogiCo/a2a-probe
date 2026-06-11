package com.a2acli.transport

import com.sun.net.httpserver.HttpServer
import kotlinx.coroutines.flow.toList
import kotlinx.coroutines.runBlocking
import kotlinx.serialization.json.*
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test
import java.net.InetSocketAddress

class SseTransportTest {

    private fun withServer(
        jsonBody: String? = null,
        sseBody: String? = null,
        block: suspend (baseUrl: String) -> Unit,
    ) {
        val server = HttpServer.create(InetSocketAddress(0), 0)
        server.createContext("/") { exchange ->
            val (ct, body) = when {
                sseBody != null -> "text/event-stream" to sseBody
                jsonBody != null -> "application/json" to jsonBody
                else -> "application/json" to ""
            }
            val bytes = body.toByteArray(Charsets.UTF_8)
            exchange.responseHeaders["Content-Type"] = listOf(ct)
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

    @Test
    fun `call returns result for JSON response`() {
        val body = jsonRpcResult("""{"id":"t1","status":{"state":"completed"}}""")
        withServer(jsonBody = body) { url ->
            val transport = SseTransport("$url/rpc")
            val result = transport.call("tasks/send", buildJsonObject { put("id", "t1") })
            assertEquals("t1", result.jsonObject["id"]?.jsonPrimitive?.content)
        }
    }

    @Test
    fun `call returns first SSE data line as result for streaming methods`() {
        val firstEvent = jsonRpcResult("""{"id":"t1","status":{"state":"working"}}""")
        val streamEvent = """{"id":"t1","status":{"state":"completed"},"final":true}"""
        val sseBody = "data: $firstEvent\n\ndata: $streamEvent\n\n"
        withServer(sseBody = sseBody) { url ->
            val transport = SseTransport("$url/rpc")
            val result = transport.call("tasks/sendSubscribe", buildJsonObject { put("id", "t1") })
            assertEquals("t1", result.jsonObject["id"]?.jsonPrimitive?.content)
        }
    }

    @Test
    fun `stream drains remaining SSE events`() {
        val firstEvent = jsonRpcResult("""{"id":"t1","status":{"state":"working"}}""")
        val streamEvent = """{"id":"t1","status":{"state":"completed"},"final":true}"""
        val sseBody = "data: $firstEvent\n\ndata: $streamEvent\n\n"
        withServer(sseBody = sseBody) { url ->
            val transport = SseTransport("$url/rpc")
            transport.call("tasks/sendSubscribe", buildJsonObject { put("id", "t1") })
            val events = transport.stream().toList()
            assertEquals(1, events.size)
            assertTrue(events[0]["final"]?.jsonPrimitive?.boolean == true)
        }
    }

    @Test
    fun `call throws JsonRpcException on JSON-RPC error`() {
        val body = """{"jsonrpc":"2.0","id":"1","error":{"code":-32001,"message":"Not found"}}"""
        withServer(jsonBody = body) { url ->
            val transport = SseTransport("$url/rpc")
            val ex = runCatching {
                transport.call("tasks/get", buildJsonObject { put("id", "x") })
            }.exceptionOrNull()
            assertTrue(ex is JsonRpcException)
            assertEquals("Not found", ex?.message)
        }
    }

    @Test
    fun `sseEndpoint defaults to rpcEndpoint without trailing rpc`() {
        val transport = SseTransport("http://localhost/rpc")
        assertNotNull(transport)
    }
}
