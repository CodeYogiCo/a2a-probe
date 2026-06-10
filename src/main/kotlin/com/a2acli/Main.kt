package com.a2acli

import com.a2acli.chat.ChatHandler
import com.a2acli.model.*
import com.a2acli.ui.*
import com.github.ajalt.clikt.core.*
import com.github.ajalt.clikt.parameters.arguments.argument
import com.github.ajalt.clikt.parameters.arguments.optional
import com.github.ajalt.clikt.parameters.options.*
import com.github.ajalt.clikt.parameters.types.choice
import com.github.ajalt.clikt.parameters.types.int
import com.github.ajalt.mordant.rendering.TextStyles.bold
import kotlinx.coroutines.runBlocking
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.put
import java.util.UUID

// ── root command ──────────────────────────────────────────────────────────────

class A2ACli : CliktCommand(
    name = "a2a-cli",
    help = "Command-line client for the A2A (Agent-to-Agent) Protocol v0.3.0",
) {
    private val server by option(
        "-s", "--server",
        help = "Server URL or name from config (e.g. http://localhost:8000 or 'myagent')",
    ).default("http://localhost:8000")

    private val transport by option(
        "-t", "--transport",
        help = "Transport protocol",
    ).choice("http", "sse", "ws", "stdio").default("http")

    private val debug by option("--debug", help = "Enable debug logging").flag()
    private val quiet by option("-q", "--quiet", help = "Suppress non-essential output").flag()

    override fun run() {
        currentContext.findOrSetObject { mutableMapOf<String, Any>() }.also { map ->
            map["server"] = server
            map["transport"] = transport
            map["debug"] = debug
            map["quiet"] = quiet
            map["sessionId"] = UUID.randomUUID().toString()
        }
    }
}

// ── shared helpers ────────────────────────────────────────────────────────────

private fun CliktCommand.sharedOptions(): Map<String, Any> =
    currentContext.findObject<MutableMap<String, Any>>() ?: mutableMapOf()

private fun buildClient(opts: Map<String, Any>): A2AClient {
    val server = opts["server"] as? String ?: "http://localhost:8000"
    val url = resolveServerUrl(server)
    return when (opts["transport"] as? String ?: "http") {
        "sse"   -> A2AClient.overSse(url)
        "ws"    -> A2AClient.overWebSocket(url)
        "stdio" -> A2AClient.overStdio()
        else    -> A2AClient.overHttp(url)
    }
}

private fun makeTextMessage(text: String, sessionId: String): Message = Message(
    role = Role.USER,
    parts = listOf(buildJsonObject { put("type", "text"); put("text", text) }),
    messageId = UUID.randomUUID().toString(),
)

// ── send command ──────────────────────────────────────────────────────────────

class SendCommand : CliktCommand(
    name = "send",
    help = "Send a new task to the agent and print the result",
) {
    private val message by argument(help = "Message text to send")
    private val taskId by option("--id", help = "Task ID (auto-generated if omitted)")
    private val stream by option("--stream", help = "Use streaming (sendSubscribe)").flag()
    private val noStream by option("--no-stream", help = "Force non-streaming even if agent supports it").flag()

    override fun run() = runBlocking {
        val opts = sharedOptions()
        val sessionId = opts["sessionId"] as String
        val client = buildClient(opts)
        val params = TaskSendParams(
            id = taskId ?: UUID.randomUUID().toString(),
            sessionId = sessionId,
            message = makeTextMessage(message, sessionId),
        )

        try {
            if (stream && !noStream) {
                val flow = client.sendSubscribe(params)
                flow.collect { event ->
                    when (event) {
                        is StreamEvent.Status   -> terminal.printStreamStatus(event.event)
                        is StreamEvent.Artifact -> terminal.printStreamArtifact(event.event)
                        is StreamEvent.Unknown  -> {}
                    }
                }
            } else {
                val task = client.sendTask(params)
                terminal.printTask(task)
            }
        } catch (e: Exception) {
            terminal.printError(e.message ?: "Failed")
            throw ProgramResult(1)
        } finally {
            client.close()
        }
    }
}

// ── get command ───────────────────────────────────────────────────────────────

class GetCommand : CliktCommand(
    name = "get",
    help = "Retrieve a task by ID",
) {
    private val taskId by argument(help = "Task ID to retrieve")
    private val historyLength by option("--history", help = "Number of history entries to include").int()

    override fun run() = runBlocking {
        val opts = sharedOptions()
        val client = buildClient(opts)
        try {
            val task = client.getTask(TaskQueryParams(id = taskId, historyLength = historyLength))
            terminal.printTask(task)
        } catch (e: Exception) {
            terminal.printError(e.message ?: "Failed")
            throw ProgramResult(1)
        } finally {
            client.close()
        }
    }
}

// ── cancel command ────────────────────────────────────────────────────────────

class CancelCommand : CliktCommand(
    name = "cancel",
    help = "Cancel a running task",
) {
    private val taskId by argument(help = "Task ID to cancel")

    override fun run() = runBlocking {
        val opts = sharedOptions()
        val client = buildClient(opts)
        try {
            client.cancelTask(TaskIdParams(id = taskId))
            terminal.printSuccess("Task $taskId cancelled.")
        } catch (e: Exception) {
            terminal.printError(e.message ?: "Failed")
            throw ProgramResult(1)
        } finally {
            client.close()
        }
    }
}

// ── watch command ─────────────────────────────────────────────────────────────

class WatchCommand : CliktCommand(
    name = "watch",
    help = "Stream live updates for an existing task",
) {
    private val taskId by argument(help = "Task ID to watch")

    override fun run() = runBlocking {
        val opts = sharedOptions()
        val client = buildClient(opts)
        try {
            client.resubscribe(TaskQueryParams(id = taskId)).collect { event ->
                when (event) {
                    is StreamEvent.Status   -> terminal.printStreamStatus(event.event)
                    is StreamEvent.Artifact -> terminal.printStreamArtifact(event.event)
                    is StreamEvent.Unknown  -> {}
                }
            }
        } catch (e: Exception) {
            terminal.printError(e.message ?: "Failed")
            throw ProgramResult(1)
        } finally {
            client.close()
        }
    }
}

// ── chat command ──────────────────────────────────────────────────────────────

class ChatCommand : CliktCommand(
    name = "chat",
    help = "Enter interactive chat mode",
) {
    override fun run() = runBlocking {
        val opts = sharedOptions()
        val server = opts["server"] as? String ?: "http://localhost:8000"
        val serverUrl = resolveServerUrl(server)
        val sessionId = opts["sessionId"] as String
        val client = buildClient(opts)
        try {
            ChatHandler(client, sessionId, serverUrl).run()
        } finally {
            client.close()
        }
    }
}

// ── stdio command ─────────────────────────────────────────────────────────────

class StdioCommand : CliktCommand(
    name = "stdio",
    help = "JSON-RPC 2.0 over stdin/stdout — ideal for CI/CD pipelines",
) {
    override fun run() = runBlocking {
        val client = A2AClient.overStdio()
        val sessionId = UUID.randomUUID().toString()
        try {
            // Read one JSON-RPC request line from stdin, execute it, write result to stdout
            val stdinText = generateSequence(::readLine)
            for (line in stdinText) {
                val trimmed = line.trim()
                if (trimmed.isEmpty()) continue
                // Pass raw lines straight through the StdioTransport — the agent
                // on the other side is the server; we are the client piping to it.
                // This mode is for direct pipe-to-agent use.
                System.out.println(trimmed)
                System.out.flush()
            }
        } finally {
            client.close()
        }
    }
}

// ── config commands ───────────────────────────────────────────────────────────

class ConfigCommand : CliktCommand(
    name = "config",
    help = "Manage server configuration (~/.a2a/config.json)",
) {
    override fun run() = Unit
}

class ConfigAddCommand : CliktCommand(
    name = "add",
    help = "Add or update a named server",
) {
    private val name by argument(help = "Alias for this server")
    private val url  by argument(help = "Server URL")
    private val transport by option("-t", "--transport").choice("http", "sse", "ws").default("http")

    override fun run() {
        val config = loadConfig()
        val updated = config.copy(
            servers = config.servers + (name to com.a2acli.model.ServerConfig(url, transport))
        )
        saveConfig(updated)
        terminal.printSuccess("Server '$name' saved → $url")
    }
}

class ConfigRemoveCommand : CliktCommand(
    name = "remove",
    help = "Remove a named server",
) {
    private val name by argument(help = "Alias to remove")

    override fun run() {
        val config = loadConfig()
        val updated = config.copy(servers = config.servers - name)
        saveConfig(updated)
        terminal.printSuccess("Server '$name' removed.")
    }
}

class ConfigListCommand : CliktCommand(
    name = "list",
    help = "List configured servers",
) {
    override fun run() {
        val config = loadConfig()
        if (config.servers.isEmpty()) {
            terminal.printInfo("No servers configured. Use: a2a-cli config add <name> <url>")
            return
        }
        terminal.println(bold("Configured servers:"))
        config.servers.forEach { (name, srv) ->
            terminal.println("  $name  ${srv.url}  [${srv.transport}]")
        }
    }
}

// ── entry point ───────────────────────────────────────────────────────────────

fun main(args: Array<String>) {
    A2ACli()
        .subcommands(
            SendCommand(),
            GetCommand(),
            CancelCommand(),
            WatchCommand(),
            ChatCommand(),
            StdioCommand(),
            ConfigCommand().subcommands(
                ConfigAddCommand(),
                ConfigRemoveCommand(),
                ConfigListCommand(),
            ),
        )
        .main(args)
}
