package com.a2acli.transport

import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.flow
import kotlinx.coroutines.withContext
import kotlinx.serialization.json.*
import java.net.URI
import java.net.http.HttpClient
import java.net.http.HttpRequest
import java.net.http.HttpResponse
import java.time.Duration
import java.util.UUID

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

    private val client: HttpClient = HttpClient.newBuilder()
        .connectTimeout(Duration.ofMillis(connectTimeoutMs))
        .build()

    private val requestTimeout: Duration = Duration.ofMillis(readTimeoutMs)

    private var pendingSseLines: List<String>? = null

    override suspend fun call(method: String, params: JsonElement): JsonElement {
        val envelope = buildJsonObject {
            put("jsonrpc", "2.0")
            put("method", method)
            put("params", params)
            put("id", UUID.randomUUID().toString())
        }

        val isStreaming = method.endsWith("/stream") ||
                method.endsWith("Subscribe") ||
                method.endsWith("/resubscribe")

        val reqBuilder = HttpRequest.newBuilder()
            .uri(URI.create(endpoint.trimEnd('/')))
            .header("Content-Type", "application/json")
            .timeout(requestTimeout)
            .POST(HttpRequest.BodyPublishers.ofString(envelope.toString()))
        if (isStreaming) reqBuilder.header("Accept", "text/event-stream")

        val response = withContext(Dispatchers.IO) {
            client.send(reqBuilder.build(), HttpResponse.BodyHandlers.ofString())
        }

        val contentType = response.headers().firstValue("content-type").orElse("")

        return when {
            "application/json" in contentType -> {
                val parsed = json.parseToJsonElement(response.body()).jsonObject
                parsed["error"]?.jsonObject?.let { err ->
                    throw JsonRpcException(
                        err["message"]?.jsonPrimitive?.content ?: "RPC error",
                        err["code"]?.jsonPrimitive?.int ?: -32000,
                        err["data"],
                    )
                }
                parsed["result"] ?: JsonNull
            }

            "text/event-stream" in contentType -> {
                val lines = response.body().lines()
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
            if (!skippedFirst) { skippedFirst = true; continue }
            val text = line.removePrefix("data:").trim()
            if (text.isEmpty() || text == "[DONE]") continue
            try { emit(json.parseToJsonElement(text).jsonObject) }
            catch (_: Exception) { emit(buildJsonObject { put("raw", text) }) }
        }
        pendingSseLines = null
    }

    override suspend fun close() {}
}
