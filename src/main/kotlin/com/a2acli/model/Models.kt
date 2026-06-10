package com.a2acli.model

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonObject

// ── Enums ────────────────────────────────────────────────────────────────────

@Serializable
enum class TaskState {
    @SerialName("submitted") SUBMITTED,
    @SerialName("working")   WORKING,
    @SerialName("completed") COMPLETED,
    @SerialName("failed")    FAILED,
    @SerialName("canceled")  CANCELED,
    @SerialName("unknown")   UNKNOWN,
}

@Serializable
enum class Role {
    @SerialName("user")      USER,
    @SerialName("assistant") ASSISTANT,
}

// ── Message parts ────────────────────────────────────────────────────────────

@Serializable
data class TextPart(
    val type: String = "text",
    val text: String,
    val metadata: JsonObject? = null,
)

@Serializable
data class FilePart(
    val type: String = "file",
    val file: FileContent,
    val metadata: JsonObject? = null,
)

@Serializable
data class FileContent(
    val name: String? = null,
    @SerialName("mimeType") val mimeType: String? = null,
    val uri: String? = null,
    val bytes: String? = null,
)

@Serializable
data class DataPart(
    val type: String = "data",
    val data: JsonObject,
    val metadata: JsonObject? = null,
)

// ── Message ──────────────────────────────────────────────────────────────────

@Serializable
data class Message(
    val role: Role,
    val parts: List<JsonElement>,
    @SerialName("messageId") val messageId: String? = null,
    val metadata: JsonObject? = null,
)

// ── Artifact ─────────────────────────────────────────────────────────────────

@Serializable
data class Artifact(
    val name: String? = null,
    val description: String? = null,
    val parts: List<JsonElement> = emptyList(),
    val index: Int = 0,
    val append: Boolean? = null,
    @SerialName("lastChunk") val lastChunk: Boolean? = null,
    val metadata: JsonObject? = null,
)

// ── Task status ───────────────────────────────────────────────────────────────

@Serializable
data class TaskStatus(
    val state: TaskState,
    val message: Message? = null,
    val timestamp: String? = null,
)

// ── Task ─────────────────────────────────────────────────────────────────────

@Serializable
data class Task(
    val id: String,
    @SerialName("sessionId") val sessionId: String? = null,
    val status: TaskStatus,
    val artifacts: List<Artifact>? = null,
    val history: List<Message>? = null,
    val metadata: JsonObject? = null,
)

// ── Params ───────────────────────────────────────────────────────────────────

@Serializable
data class TaskSendParams(
    val id: String,
    @SerialName("sessionId") val sessionId: String? = null,
    val message: Message,
    @SerialName("acceptedOutputModes") val acceptedOutputModes: List<String>? = null,
    @SerialName("pushNotification") val pushNotification: PushNotificationConfig? = null,
    @SerialName("historyLength") val historyLength: Int? = null,
    val metadata: JsonObject? = null,
)

@Serializable
data class TaskQueryParams(
    val id: String,
    @SerialName("historyLength") val historyLength: Int? = null,
    val metadata: JsonObject? = null,
)

@Serializable
data class TaskIdParams(
    val id: String,
    val metadata: JsonObject? = null,
)

@Serializable
data class PushNotificationConfig(
    val id: String,
    val url: String,
    val token: String? = null,
    val authentication: JsonObject? = null,
)

// ── Streaming events ──────────────────────────────────────────────────────────

@Serializable
data class TaskStatusUpdateEvent(
    val id: String,
    val status: TaskStatus,
    val final: Boolean = false,
    val metadata: JsonObject? = null,
)

@Serializable
data class TaskArtifactUpdateEvent(
    val id: String,
    val artifact: Artifact,
    val metadata: JsonObject? = null,
)

// ── JSON-RPC envelopes ────────────────────────────────────────────────────────

@Serializable
data class JsonRpcRequest(
    val jsonrpc: String = "2.0",
    val method: String,
    val params: JsonElement,
    val id: String,
)

@Serializable
data class JsonRpcResponse(
    val jsonrpc: String = "2.0",
    val result: JsonElement? = null,
    val error: JsonRpcError? = null,
    val id: JsonElement? = null,
)

@Serializable
data class JsonRpcError(
    val code: Int,
    val message: String,
    val data: JsonElement? = null,
)

// ── Agent card ────────────────────────────────────────────────────────────────

@Serializable
data class AgentCard(
    val name: String? = null,
    val description: String? = null,
    val url: String? = null,
    val version: String? = null,
    val capabilities: AgentCapabilities? = null,
    val skills: List<AgentSkill>? = null,
)

@Serializable
data class AgentCapabilities(
    val streaming: Boolean? = null,
    val pushNotifications: Boolean? = null,
    val stateTransitionHistory: Boolean? = null,
)

@Serializable
data class AgentSkill(
    val id: String,
    val name: String,
    val description: String? = null,
    val tags: List<String>? = null,
)

// ── Config ────────────────────────────────────────────────────────────────────

@Serializable
data class ServerConfig(
    val url: String,
    val transport: String = "http",
)

@Serializable
data class CliConfig(
    val servers: Map<String, ServerConfig> = emptyMap(),
    @SerialName("default_server") val defaultServer: String? = null,
)
