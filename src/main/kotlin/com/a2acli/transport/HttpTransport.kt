package com.a2acli.transport

import io.ktor.client.*
import io.ktor.client.engine.cio.*
import io.ktor.client.plugins.*
import io.ktor.client.plugins.contentnegotiation.*
import io.ktor.client.request.*
import io.ktor.client.statement.*
import io.ktor.http.*
import io.ktor.serialization.kotlinx.json.*
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.flow
import kotlinx.serialization.json.*
import java.util.UUID

/**
 * HTTP transport for JSON-RPC 2.0.
 *
 * Handles both plain application/json responses and merged text/event-stream
 * responses (where the first SSE data line is the JSON-RPC result and subsequent
 * lines are streamed via [stream]).
 */
class HttpTransport(
    private val endpoint: String,
    connectTimeoutMs: Long = 10_000,
    readTimeoutMs: Long = 90_000,
) : JsonRpcTransport {

    private val json = Json {
        ignoreUnknownKeys = true
        isLenient = true
        encodeDefaults = false
        explicitNulls = false
    }

    private val client = HttpClient(CIO) {
        install(ContentNegotiation) { json(json) }
        install(HttpTimeout) {
            connectTimeoutMillis = connectTimeoutMs
            requestTimeoutMillis = readTimeoutMs
            socketTimeoutMillis = readTimeoutMs
        }
    }

    // Holds the raw SSE body text for merged-stream responses.
    private var pendingSseLines: List<String>? = null

    override suspend fun call(method: String, params: JsonElement): JsonElement {
        val requestId = UUID.randomUUID().toString()
        val envelope = buildJsonObject {
            put("jsonrpc", "2.0")
            put("method", method)
            put("params", params)
            put("id", requestId)
        }

        val response: HttpResponse = client.post(endpoint.trimEnd('/')) {
            contentType(ContentType.Application.Json)
            setBody(envelope.toString())
        }

        val contentType = response.contentType()?.withoutParameters()

        return when {
            contentType == ContentType.Application.Json -> {
                val body = response.bodyAsText()
                val parsed = json.parseToJsonElement(body).jsonObject
                parsed["error"]?.jsonObject?.let { err ->
                    throw JsonRpcException(
                        err["message"]?.jsonPrimitive?.content ?: "RPC error",
                        err["code"]?.jsonPrimitive?.int ?: -32000,
                        err["data"],
                    )
                }
                parsed["result"] ?: JsonNull
            }

            contentType?.match("text", "event-stream") == true -> {
                val body = response.bodyAsText()
                val lines = body.lines()
                pendingSseLines = lines

                val firstData = lines.firstOrNull { it.startsWith("data:") }
                    ?: throw JsonRpcException("Empty SSE stream")

                val first = json.parseToJsonElement(firstData.removePrefix("data:").trim()).jsonObject
                first["error"]?.jsonObject?.let { err ->
                    throw JsonRpcException(
                        err["message"]?.jsonPrimitive?.content ?: "RPC error",
                        err["code"]?.jsonPrimitive?.int ?: -32000,
                        err["data"],
                    )
                }
                first["result"] ?: JsonNull
            }

            else -> throw JsonRpcException("Unsupported Content-Type: $contentType")
        }
    }

    override fun stream(): Flow<JsonObject> = flow {
        val lines = pendingSseLines
            ?: throw IllegalStateException("stream() called before a merged SSE call")

        var skippedFirst = false
        for (line in lines) {
            if (!line.startsWith("data:")) continue
            if (!skippedFirst) {
                skippedFirst = true
                continue
            }
            val text = line.removePrefix("data:").trim()
            if (text.isEmpty() || text == "[DONE]") continue
            try {
                emit(json.parseToJsonElement(text).jsonObject)
            } catch (_: Exception) {
                emit(buildJsonObject { put("raw", text) })
            }
        }
        pendingSseLines = null
    }

    override suspend fun close() {
        client.close()
    }
}
