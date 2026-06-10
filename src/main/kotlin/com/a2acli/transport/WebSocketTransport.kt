package com.a2acli.transport

import io.ktor.client.*
import io.ktor.client.engine.cio.*
import io.ktor.client.plugins.websocket.*
import io.ktor.websocket.*
import kotlinx.coroutines.channels.ReceiveChannel
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.flow
import kotlinx.serialization.json.*
import java.util.UUID

/**
 * WebSocket transport for JSON-RPC 2.0.
 *
 * Maintains a single persistent WebSocket connection. Each [call] sends a
 * JSON-RPC request frame and reads exactly one response frame. [stream]
 * reads subsequent frames as a Flow.
 */
class WebSocketTransport(
    private val url: String,
    connectTimeoutMs: Long = 10_000,
) : JsonRpcTransport {

    private val json = Json {
        ignoreUnknownKeys = true
        isLenient = true
        encodeDefaults = false
        explicitNulls = false
    }

    private val client = HttpClient(CIO) {
        install(WebSockets)
    }

    private var session: WebSocketSession? = null
    private var incoming: ReceiveChannel<Frame>? = null

    private suspend fun ensureConnected(): WebSocketSession {
        val existing = session
        if (existing != null) return existing
        val ws = client.webSocketSession(url)
        session = ws
        incoming = ws.incoming
        return ws
    }

    override suspend fun call(method: String, params: JsonElement): JsonElement {
        val ws = ensureConnected()
        val requestId = UUID.randomUUID().toString()
        val envelope = buildJsonObject {
            put("jsonrpc", "2.0")
            put("method", method)
            put("params", params)
            put("id", requestId)
        }
        ws.send(Frame.Text(envelope.toString()))

        // Read one response frame
        val frame = ws.incoming.receive()
        val text = (frame as Frame.Text).readText()
        val parsed = json.parseToJsonElement(text).jsonObject
        parsed["error"]?.jsonObject?.let { err ->
            throw JsonRpcException(
                err["message"]?.jsonPrimitive?.content ?: "RPC error",
                err["code"]?.jsonPrimitive?.int ?: -32000,
                err["data"],
            )
        }
        return parsed["result"] ?: JsonNull
    }

    override fun stream(): Flow<JsonObject> = flow {
        val ws = session ?: throw IllegalStateException("stream() called before connect")
        for (frame in ws.incoming) {
            if (frame is Frame.Text) {
                val text = frame.readText()
                try {
                    emit(json.parseToJsonElement(text).jsonObject)
                } catch (_: Exception) {
                    emit(buildJsonObject { put("raw", text) })
                }
            }
        }
    }

    override suspend fun close() {
        session?.close()
        session = null
        client.close()
    }
}
