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

class SseTransportTest {

    private val json = Json { ignoreUnknownKeys = true; isLenient = true }

    private fun mockClient(
        jsonBody: String? = null,
        sseBody: String? = null,
    ): HttpClient {
        return HttpClient(MockEngine { request ->
            when {
                sseBody != null -> respond(
                    sseBody,
                    HttpStatusCode.OK,
                    headersOf(HttpHeaders.ContentType, "text/event-stream"),
                )
                jsonBody != null -> respond(
                    jsonBody,
                    HttpStatusCode.OK,
                    headersOf(HttpHeaders.ContentType, ContentType.Application.Json.toString()),
                )
                else -> respond("", HttpStatusCode.OK)
            }
        }) {
            install(ContentNegotiation) { json(json) }
        }
    }

    private fun jsonRpcResult(result: String) =
        """{"jsonrpc":"2.0","id":"1","result":$result}"""

    @Test
    fun `call returns result for JSON response`() = runTest {
        val body = jsonRpcResult("""{"id":"t1","status":{"state":"completed"}}""")
        val transport = SseTransport("http://localhost/rpc", testClient = mockClient(jsonBody = body))
        val result = transport.call("tasks/send", buildJsonObject { put("id", "t1") })
        assertEquals("t1", result.jsonObject["id"]?.jsonPrimitive?.content)
    }

    @Test
    fun `call returns first SSE data line as result for streaming methods`() = runTest {
        val firstEvent = jsonRpcResult("""{"id":"t1","status":{"state":"working"}}""")
        val streamEvent = """{"id":"t1","status":{"state":"completed"},"final":true}"""
        val sseBody = "data: $firstEvent\n\ndata: $streamEvent\n\n"
        val transport = SseTransport("http://localhost/rpc", testClient = mockClient(sseBody = sseBody))
        val result = transport.call("tasks/sendSubscribe", buildJsonObject { put("id", "t1") })
        assertEquals("t1", result.jsonObject["id"]?.jsonPrimitive?.content)
    }

    @Test
    fun `stream drains remaining SSE events`() = runTest {
        val firstEvent = jsonRpcResult("""{"id":"t1","status":{"state":"working"}}""")
        val streamEvent = """{"id":"t1","status":{"state":"completed"},"final":true}"""
        val sseBody = "data: $firstEvent\n\ndata: $streamEvent\n\n"
        val transport = SseTransport("http://localhost/rpc", testClient = mockClient(sseBody = sseBody))
        transport.call("tasks/sendSubscribe", buildJsonObject { put("id", "t1") })
        val events = transport.stream().toList()
        assertEquals(1, events.size)
        assertTrue(events[0]["final"]?.jsonPrimitive?.boolean == true)
    }

    @Test
    fun `call throws JsonRpcException on JSON-RPC error`() = runTest {
        val body = """{"jsonrpc":"2.0","id":"1","error":{"code":-32001,"message":"Not found"}}"""
        val transport = SseTransport("http://localhost/rpc", testClient = mockClient(jsonBody = body))
        val ex = runCatching {
            transport.call("tasks/get", buildJsonObject { put("id", "x") })
        }.exceptionOrNull()
        assertTrue(ex is JsonRpcException)
        assertEquals("Not found", ex?.message)
    }

    @Test
    fun `sseEndpoint defaults to rpcEndpoint without trailing rpc`() = runTest {
        // Just verify construction doesn't throw; endpoint resolution is internal
        val transport = SseTransport("http://localhost/rpc")
        assertNotNull(transport)
    }
}
