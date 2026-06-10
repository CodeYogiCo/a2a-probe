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
 * SSE transport for JSON-RPC 2.0.
 *
 * Sends RPC calls via POST to [rpcEndpoint]. For streaming subscriptions
 * (tasks/sendSubscribe, tasks/resubscribe) it drains the event-stream that
 * comes back in [stream]. For standalone SSE polling it opens a GET to
 * [sseEndpoint] (defaults to [rpcEndpoint] with "/rpc" stripped).
 */
class SseTransport(
    private val rpcEndpoint: String,
    sseEndpoint: String? = null,
    connectTimeoutMs: Long = 10_000,
    readTimeoutMs: Long = 90_000,
) : JsonRpcTransport {

    private val sseEndpoint = sseEndpoint
        ?: rpcEndpoint.trimEnd('/').removeSuffix("/rpc")

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

    private var pendingSseBody: String? = null
    private var skippedFirstSseLine = false

    override suspend fun call(method: String, params: JsonElement): JsonElement {
        val requestId = UUID.randomUUID().toString()
        val envelope = buildJsonObject {
            put("jsonrpc", "2.0")
            put("method", method)
            put("params", params)
            put("id", requestId)
        }

        val targetUrl = if (method in setOf("tasks/sendSubscribe", "tasks/resubscribe")) {
            sseEndpoint.trimEnd('/')
        } else {
            rpcEndpoint.trimEnd('/')
        }

        val response: HttpResponse = client.post(targetUrl) {
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
                pendingSseBody = response.bodyAsText()
                skippedFirstSseLine = false

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
            // Drain pending merged stream (skipping the first data line already consumed)
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
            // Standalone GET to sseEndpoint
            val response: HttpResponse = client.get(sseEndpoint)
            val text = response.bodyAsText()
            for (line in text.lines()) {
                if (!line.startsWith("data:")) continue
                val data = line.removePrefix("data:").trim()
                if (data.isEmpty() || data == "[DONE]") continue
                try { emit(json.parseToJsonElement(data).jsonObject) }
                catch (_: Exception) { emit(buildJsonObject { put("raw", data) }) }
            }
        }
    }

    override suspend fun close() {
        client.close()
    }
}
