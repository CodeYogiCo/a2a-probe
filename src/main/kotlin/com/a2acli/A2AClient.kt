package com.a2acli

import com.a2acli.model.*
import com.a2acli.transport.*
import io.ktor.client.*
import io.ktor.client.engine.cio.*
import io.ktor.client.plugins.*
import io.ktor.client.request.*
import io.ktor.client.statement.*
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.map
import kotlinx.serialization.encodeToString
import kotlinx.serialization.json.*

/**
 * Transport-agnostic high-level A2A client.
 *
 * Mirrors the Python a2a_cli.A2AClient, adapting Python's pydantic
 * model_dump/model_validate pattern to kotlinx.serialization encode/decode.
 */
class A2AClient(private val transport: JsonRpcTransport) {

    private val json = Json {
        ignoreUnknownKeys = true
        isLenient = true
        encodeDefaults = false
        explicitNulls = false
    }

    // ── factory helpers ────────────────────────────────────────────────────

    companion object {
        fun overHttp(endpoint: String) =
            A2AClient(HttpTransport(endpoint))

        fun overSse(endpoint: String, sseEndpoint: String? = null) =
            A2AClient(SseTransport(endpoint, sseEndpoint))

        fun overWebSocket(url: String) =
            A2AClient(WebSocketTransport(url))

        fun overStdio() =
            A2AClient(StdioTransport())
    }

    // ── basic RPC wrappers ─────────────────────────────────────────────────

    suspend fun sendTask(params: TaskSendParams): Task {
        val raw = transport.call("tasks/send", json.encodeToJsonElement(params))
        return json.decodeFromJsonElement(raw)
    }

    suspend fun getTask(params: TaskQueryParams): Task {
        val raw = transport.call("tasks/get", json.encodeToJsonElement(params))
        return json.decodeFromJsonElement(raw)
    }

    suspend fun cancelTask(params: TaskIdParams) {
        transport.call("tasks/cancel", json.encodeToJsonElement(params))
    }

    suspend fun setPushNotification(params: PushNotificationConfig): PushNotificationConfig {
        val raw = transport.call("tasks/pushNotification/set", json.encodeToJsonElement(params))
        return json.decodeFromJsonElement(raw)
    }

    suspend fun getPushNotification(params: TaskIdParams): PushNotificationConfig {
        val raw = transport.call("tasks/pushNotification/get", json.encodeToJsonElement(params))
        return json.decodeFromJsonElement(raw)
    }

    // ── streaming ──────────────────────────────────────────────────────────

    /**
     * Send a task and subscribe to its event stream.
     * Emits [TaskStatusUpdateEvent] or [TaskArtifactUpdateEvent] instances.
     */
    suspend fun sendSubscribe(params: TaskSendParams): Flow<StreamEvent> {
        transport.call("tasks/sendSubscribe", json.encodeToJsonElement(params))
        return transport.stream().map { coerceStreamEvent(it) }
    }

    suspend fun resubscribe(params: TaskQueryParams): Flow<StreamEvent> {
        transport.call("tasks/resubscribe", json.encodeToJsonElement(params))
        return transport.stream().map { coerceStreamEvent(it) }
    }

    // ── agent card ─────────────────────────────────────────────────────────

    /**
     * Fetches the agent card from /.well-known/agent.json relative to [baseUrl].
     */
    suspend fun fetchAgentCard(baseUrl: String): AgentCard? {
        val cardUrl = baseUrl.trimEnd('/') + "/.well-known/agent.json"
        return try {
            val httpClient = HttpClient(CIO) {
                install(HttpTimeout) { requestTimeoutMillis = 10_000 }
            }
            val response: HttpResponse = httpClient.get(cardUrl)
            httpClient.close()
            json.decodeFromString<AgentCard>(response.bodyAsText())
        } catch (_: Exception) {
            null
        }
    }

    suspend fun close() = transport.close()

    // ── event normalisation ────────────────────────────────────────────────

    private fun coerceStreamEvent(raw: JsonObject): StreamEvent {
        // HTTP/WS transports wrap events: {"method":"tasks/event","params":{…}}
        val payload = if (raw["method"]?.jsonPrimitive?.content == "tasks/event") {
            raw["params"]?.jsonObject ?: raw
        } else raw

        return when {
            payload.containsKey("status") -> StreamEvent.Status(
                json.decodeFromJsonElement(
                    JsonObject(payload.filterKeys { it != "type" })
                )
            )
            payload.containsKey("artifact") -> StreamEvent.Artifact(
                json.decodeFromJsonElement(
                    JsonObject(payload.filterKeys { it != "type" })
                )
            )
            else -> StreamEvent.Unknown(payload)
        }
    }
}

sealed class StreamEvent {
    data class Status(val event: TaskStatusUpdateEvent) : StreamEvent()
    data class Artifact(val event: TaskArtifactUpdateEvent) : StreamEvent()
    data class Unknown(val raw: JsonObject) : StreamEvent()
}
