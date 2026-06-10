package com.a2acli

import com.a2acli.model.CliConfig
import com.a2acli.model.ServerConfig
import kotlinx.serialization.json.Json
import java.io.File

private val json = Json {
    ignoreUnknownKeys = true
    isLenient = true
    prettyPrint = true
}

private val configFile: File
    get() = File(System.getProperty("user.home"), ".a2a/config.json")

fun loadConfig(): CliConfig {
    if (!configFile.exists()) return CliConfig()
    return try {
        json.decodeFromString(configFile.readText())
    } catch (_: Exception) {
        CliConfig()
    }
}

fun saveConfig(config: CliConfig) {
    configFile.parentFile.mkdirs()
    configFile.writeText(json.encodeToString(CliConfig.serializer(), config))
}

fun resolveServerUrl(nameOrUrl: String): String {
    if (nameOrUrl.startsWith("http://") || nameOrUrl.startsWith("https://") ||
        nameOrUrl.startsWith("ws://") || nameOrUrl.startsWith("wss://")
    ) return nameOrUrl

    val config = loadConfig()
    return config.servers[nameOrUrl]?.url
        ?: error("Unknown server '$nameOrUrl'. Add it with: a2a-cli config add $nameOrUrl <url>")
}
