package com.a2acli.transport

import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonObject

interface JsonRpcTransport {
    suspend fun call(method: String, params: JsonElement): JsonElement
    fun stream(): kotlinx.coroutines.flow.Flow<JsonObject>
    suspend fun close()
}

class JsonRpcException(message: String, val code: Int = -32000, val data: JsonElement? = null) :
    Exception(message)
