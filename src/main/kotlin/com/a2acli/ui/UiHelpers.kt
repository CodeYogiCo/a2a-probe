package com.a2acli.ui

import com.a2acli.model.*
import com.github.ajalt.mordant.rendering.TextColors.*
import com.github.ajalt.mordant.rendering.TextStyles.*
import com.github.ajalt.mordant.terminal.Terminal
import kotlinx.serialization.json.*

val terminal = Terminal()

fun Terminal.printWelcomeBanner(sessionId: String) {
    println()
    println(bold("╔══════════════════════════════════════╗"))
    println(bold("║        A2A CLI — Kotlin Edition       ║"))
    println(bold("║    Agent-to-Agent Protocol v0.3.0     ║"))
    println(bold("╚══════════════════════════════════════╝"))
    println(dim("  Session: $sessionId"))
    println(dim("  Type 'help' or '/help' for commands"))
    println()
}

fun Terminal.printTask(task: Task) {
    val stateColor = when (task.status.state) {
        TaskState.COMPLETED -> green
        TaskState.FAILED    -> red
        TaskState.CANCELED  -> yellow
        TaskState.WORKING   -> cyan
        else                -> white
    }
    println(bold("Task ") + white(task.id))
    println("  Status: " + stateColor(task.status.state.name.lowercase()))
    task.status.message?.let { printMessage(it, indent = "  ") }
    task.artifacts?.forEachIndexed { i, artifact ->
        println(dim("  ─── artifact[$i]: ${artifact.name ?: "unnamed"} ───"))
        printArtifactParts(artifact.parts)
    }
}

fun Terminal.printMessage(message: Message, indent: String = "") {
    val prefix = when (message.role) {
        Role.USER      -> bold(blue("You:      "))
        Role.ASSISTANT -> bold(green("Agent:    "))
    }
    val text = extractText(message.parts)
    println("$indent$prefix$text")
}

fun Terminal.printStreamStatus(event: TaskStatusUpdateEvent) {
    val stateColor = when (event.status.state) {
        TaskState.COMPLETED -> green
        TaskState.FAILED    -> red
        TaskState.CANCELED  -> yellow
        TaskState.WORKING   -> cyan
        else                -> white
    }
    print("\r" + dim("[") + stateColor(event.status.state.name.lowercase()) + dim("]") + " ")
    event.status.message?.let {
        print(extractText(it.parts))
    }
    if (event.final) println()
}

fun Terminal.printStreamArtifact(event: TaskArtifactUpdateEvent) {
    if (event.artifact.index == 0 && event.artifact.append != true) println()
    printArtifactParts(event.artifact.parts)
}

private fun Terminal.printArtifactParts(parts: List<JsonElement>) {
    for (part in parts) {
        val obj = part.jsonObject
        when (obj["type"]?.jsonPrimitive?.content) {
            "text" -> print(obj["text"]?.jsonPrimitive?.content ?: "")
            "file" -> {
                val file = obj["file"]?.jsonObject
                val uri = file?.get("uri")?.jsonPrimitive?.content
                val name = file?.get("name")?.jsonPrimitive?.content
                println(cyan("[file: ${name ?: uri ?: "unknown"}]"))
            }
            "data" -> println(dim(obj["data"].toString()))
            else   -> println(dim(part.toString()))
        }
    }
}

fun extractText(parts: List<JsonElement>): String =
    parts.joinToString("") { part ->
        val obj = runCatching { part.jsonObject }.getOrNull() ?: return@joinToString part.toString()
        when (obj["type"]?.jsonPrimitive?.content) {
            "text" -> obj["text"]?.jsonPrimitive?.content ?: ""
            else   -> obj.toString()
        }
    }

fun Terminal.printError(msg: String) = println(red("Error: $msg"))
fun Terminal.printInfo(msg: String)  = println(dim(msg))
fun Terminal.printSuccess(msg: String) = println(green(msg))
