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

class SseTransport(
    private val rpcEndpoint: String,
    sseEndpoint: String? = null,
    connectTimeoutMs: Long = 10_000,
    readTimeoutMs: Long = 90_000,
) : JsonRpcTransport {

    private val sseEndpoint: String = sseEndpoint
        ?: rpcEndpoint.trimEnd('/').removeSuffix("/rpc")

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

    private var pendingSseBody: String? = null

    override suspend fun call(method: String, params: JsonElement): JsonElement {
        val envelope = buildJsonObject {
            put("jsonrpc", "2.0")
            put("method", method)
            put("params", params)
            put("id", UUID.randomUUID().toString())
        }

        val targetUrl = if (method in setOf("tasks/sendSubscribe", "tasks/resubscribe")) {
            sseEndpoint.trimEnd('/')
        } else {
            rpcEndpoint.trimEnd('/')
        }

        val response = withContext(Dispatchers.IO) {
            client.send(
                HttpRequest.newBuilder()
                    .uri(URI.create(targetUrl))
                    .header("Content-Type", "application/json")
                    .timeout(requestTimeout)
                    .POST(HttpRequest.BodyPublishers.ofString(envelope.toString()))
                    .build(),
                HttpResponse.BodyHandlers.ofString()
            )
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
                pendingSseBody = response.body()
                val firstData = pendingSseBody!!.lines().firstOrNull { it.startsWith("data:") }
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
        val body = pendingSseBody
        if (body != null) {
            var skippedFirst = false
            for (line in body.lines()) {
                if (!line.startsWith("data:")) continue
                if (!skippedFirst) { skippedFirst = true; continue }
                val text = line.removePrefix("data:").trim()
                if (text.isEmpty() || text == "[DONE]") continue
                try { emit(json.parseToJsonElement(text).jsonObject) }
                catch (_: Exception) { emit(buildJsonObject { put("raw", text) }) }
            }
            pendingSseBody = null
        } else {
            val response = withContext(Dispatchers.IO) {
                client.send(
                    HttpRequest.newBuilder()
                        .uri(URI.create(sseEndpoint))
                        .timeout(requestTimeout)
                        .GET()
                        .build(),
                    HttpResponse.BodyHandlers.ofString()
                )
            }
            for (line in response.body().lines()) {
                if (!line.startsWith("data:")) continue
                val data = line.removePrefix("data:").trim()
                if (data.isEmpty() || data == "[DONE]") continue
                try { emit(json.parseToJsonElement(data).jsonObject) }
                catch (_: Exception) { emit(buildJsonObject { put("raw", data) }) }
            }
        }
    }

    override suspend fun close() {}
}
