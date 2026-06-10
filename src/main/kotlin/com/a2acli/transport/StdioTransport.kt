package com.a2acli.transport

import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.flow
import kotlinx.coroutines.withContext
import kotlinx.serialization.json.*
import java.io.BufferedReader
import java.io.InputStreamReader
import java.io.PrintWriter
import java.util.UUID

/**
 * STDIO transport for JSON-RPC 2.0.
 *
 * Writes JSON-RPC requests to stdout and reads responses from stdin.
 * Designed for pipeline / CI use: pipe a JSON-RPC request in, receive the
 * response out. Each call writes one newline-delimited JSON object.
 */
class StdioTransport : JsonRpcTransport {

    private val json = Json {
        ignoreUnknownKeys = true
        isLenient = true
        encodeDefaults = false
        explicitNulls = false
    }

    private val reader = BufferedReader(InputStreamReader(System.`in`))
    private val writer = PrintWriter(System.out, true)

    override suspend fun call(method: String, params: JsonElement): JsonElement =
        withContext(Dispatchers.IO) {
            val requestId = UUID.randomUUID().toString()
            val envelope = buildJsonObject {
                put("jsonrpc", "2.0")
                put("method", method)
                put("params", params)
                put("id", requestId)
            }
            writer.println(envelope.toString())
            writer.flush()

            val line = reader.readLine()
                ?: throw JsonRpcException("EOF on stdin — server closed the pipe")

            val parsed = json.parseToJsonElement(line).jsonObject
            parsed["error"]?.jsonObject?.let { err ->
                throw JsonRpcException(
                    err["message"]?.jsonPrimitive?.content ?: "RPC error",
                    err["code"]?.jsonPrimitive?.int ?: -32000,
                    err["data"],
                )
            }
            parsed["result"] ?: JsonNull
        }

    override fun stream(): Flow<JsonObject> = flow {
        withContext(Dispatchers.IO) {
            var line = reader.readLine()
            while (line != null) {
                val trimmed = line.trim()
                if (trimmed.isNotEmpty()) {
                    try {
                        emit(json.parseToJsonElement(trimmed).jsonObject)
                    } catch (_: Exception) {
                        emit(buildJsonObject { put("raw", trimmed) })
                    }
                }
                line = reader.readLine()
            }
        }
    }

    override suspend fun close() {
        // stdin/stdout are managed by the OS; nothing to close here
    }
}
