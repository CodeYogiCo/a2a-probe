package com.a2acli.transport

import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.channels.Channel
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.flow
import kotlinx.coroutines.withContext
import kotlinx.serialization.json.*
import java.net.URI
import java.net.http.HttpClient
import java.net.http.WebSocket
import java.util.UUID
import java.util.concurrent.CompletableFuture
import java.util.concurrent.CompletionStage

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

    private val httpClient: HttpClient = HttpClient.newHttpClient()
    private val messageChannel = Channel<String>(Channel.UNLIMITED)
    private val textBuffer = StringBuilder()
    private var webSocket: WebSocket? = null

    private val listener = object : WebSocket.Listener {
        override fun onText(ws: WebSocket, data: CharSequence, last: Boolean): CompletionStage<*> {
            textBuffer.append(data)
            if (last) {
                messageChannel.trySend(textBuffer.toString())
                textBuffer.clear()
            }
            ws.request(1)
            return CompletableFuture.completedFuture(null)
        }

        override fun onClose(ws: WebSocket, statusCode: Int, reason: String): CompletionStage<*> {
            messageChannel.close()
            return CompletableFuture.completedFuture(null)
        }

        override fun onError(ws: WebSocket, error: Throwable) {
            messageChannel.close(Exception(error))
        }
    }

    private suspend fun ensureConnected(): WebSocket {
        webSocket?.let { return it }
        val ws = withContext(Dispatchers.IO) {
            httpClient.newWebSocketBuilder()
                .buildAsync(URI.create(url), listener)
                .get()
        }
        ws.request(1)
        webSocket = ws
        return ws
    }

    override suspend fun call(method: String, params: JsonElement): JsonElement {
        val ws = ensureConnected()
        val envelope = buildJsonObject {
            put("jsonrpc", "2.0")
            put("method", method)
            put("params", params)
            put("id", UUID.randomUUID().toString())
        }
        withContext(Dispatchers.IO) {
            ws.sendText(envelope.toString(), true).get()
        }
        val text = messageChannel.receive()
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
        for (text in messageChannel) {
            try { emit(json.parseToJsonElement(text).jsonObject) }
            catch (_: Exception) { emit(buildJsonObject { put("raw", text) }) }
        }
    }

    override suspend fun close() {
        withContext(Dispatchers.IO) {
            webSocket?.sendClose(WebSocket.NORMAL_CLOSURE, "")?.get()
        }
        webSocket = null
        messageChannel.close()
    }
}
