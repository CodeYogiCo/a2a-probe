package com.a2acli.chat

import com.a2acli.A2AClient
import com.a2acli.StreamEvent
import com.a2acli.model.*
import com.a2acli.ui.*
import kotlinx.coroutines.*
import kotlinx.coroutines.flow.catch
import org.jline.reader.LineReaderBuilder
import org.jline.reader.UserInterruptException
import org.jline.reader.EndOfFileException
import org.jline.terminal.TerminalBuilder
import java.util.UUID

class ChatHandler(
    private val client: A2AClient,
    private val sessionId: String,
    private val serverUrl: String,
) {
    private val scope = CoroutineScope(Dispatchers.IO + SupervisorJob())

    suspend fun run() {
        val jlineTerminal = TerminalBuilder.builder().system(true).build()
        val reader = LineReaderBuilder.builder()
            .terminal(jlineTerminal)
            .appName("a2a-cli")
            .build()

        terminal.printWelcomeBanner(sessionId)
        terminal.printInfo("Connected to: $serverUrl")

        val agentCard = client.fetchAgentCard(serverUrl)
        agentCard?.let {
            terminal.printInfo("Agent: ${it.name ?: "unknown"} ${it.version?.let { v -> "v$v" } ?: ""}")
            it.description?.let { d -> terminal.printInfo(d) }
        }
        terminal.printInfo("")

        try {
            while (true) {
                val line = try {
                    reader.readLine("> ")
                } catch (_: UserInterruptException) {
                    terminal.printInfo("\nUse 'exit' or Ctrl-D to quit.")
                    continue
                } catch (_: EndOfFileException) {
                    break
                } ?: break

                val trimmed = line.trim()
                if (trimmed.isEmpty()) continue

                when {
                    trimmed.lowercase() in listOf("exit", "quit", "/exit", "/quit") -> break
                    trimmed.lowercase() in listOf("help", "/help") -> printHelp()
                    trimmed.startsWith("/get ") -> handleGet(trimmed.removePrefix("/get ").trim())
                    trimmed.startsWith("/cancel ") -> handleCancel(trimmed.removePrefix("/cancel ").trim())
                    trimmed.startsWith("/watch ") -> handleWatch(trimmed.removePrefix("/watch ").trim())
                    else -> handleSend(trimmed, agentCard)
                }
            }
        } finally {
            jlineTerminal.close()
            scope.cancel()
        }
    }

    private suspend fun handleSend(message: String, agentCard: AgentCard?) {
        val params = TaskSendParams(
            id = UUID.randomUUID().toString(),
            sessionId = sessionId,
            message = Message(
                role = Role.USER,
                parts = listOf(
                    kotlinx.serialization.json.buildJsonObject {
                        put("type", "text")
                        put("text", message)
                    }
                ),
            ),
        )

        val canStream = agentCard?.capabilities?.streaming == true

        if (canStream) {
            try {
                client.sendSubscribe(params)
                    .catch { e -> terminal.printError(e.message ?: "Stream error") }
                    .collect { event ->
                        when (event) {
                            is StreamEvent.Status   -> terminal.printStreamStatus(event.event)
                            is StreamEvent.Artifact -> terminal.printStreamArtifact(event.event)
                            is StreamEvent.Unknown  -> { /* ignore unknown events */ }
                        }
                    }
            } catch (e: Exception) {
                terminal.printError(e.message ?: "Failed to stream task")
            }
        } else {
            try {
                val task = client.sendTask(params)
                terminal.printTask(task)
            } catch (e: Exception) {
                terminal.printError(e.message ?: "Failed to send task")
            }
        }
    }

    private suspend fun handleGet(taskId: String) {
        try {
            val task = client.getTask(TaskQueryParams(id = taskId))
            terminal.printTask(task)
        } catch (e: Exception) {
            terminal.printError(e.message ?: "Failed to get task $taskId")
        }
    }

    private suspend fun handleCancel(taskId: String) {
        try {
            client.cancelTask(TaskIdParams(id = taskId))
            terminal.printSuccess("Task $taskId cancelled.")
        } catch (e: Exception) {
            terminal.printError(e.message ?: "Failed to cancel task $taskId")
        }
    }

    private suspend fun handleWatch(taskId: String) {
        try {
            client.resubscribe(TaskQueryParams(id = taskId))
                .catch { e -> terminal.printError(e.message ?: "Stream error") }
                .collect { event ->
                    when (event) {
                        is StreamEvent.Status   -> terminal.printStreamStatus(event.event)
                        is StreamEvent.Artifact -> terminal.printStreamArtifact(event.event)
                        is StreamEvent.Unknown  -> { /* ignore */ }
                    }
                }
        } catch (e: Exception) {
            terminal.printError(e.message ?: "Failed to watch task $taskId")
        }
    }

    private fun printHelp() {
        terminal.println("""
            Commands:
              <message>          Send a message to the agent
              /get <id>          Retrieve task status by ID
              /cancel <id>       Cancel a running task
              /watch <id>        Stream updates for a task
              /help              Show this help
              exit               Quit
        """.trimIndent())
    }
}
