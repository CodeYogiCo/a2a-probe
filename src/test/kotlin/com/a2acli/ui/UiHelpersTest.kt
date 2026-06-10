package com.a2acli.ui

import kotlinx.serialization.json.buildJsonArray
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.put
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test

class UiHelpersTest {

    // ── extractText ───────────────────────────────────────────────────────────

    @Test
    fun `extractText returns text from kind=text part`() {
        val parts = listOf(buildJsonObject { put("kind", "text"); put("text", "Hello world") })
        assertEquals("Hello world", extractText(parts))
    }

    @Test
    fun `extractText falls back to type=text for backward compatibility`() {
        val parts = listOf(buildJsonObject { put("type", "text"); put("text", "Fallback") })
        assertEquals("Fallback", extractText(parts))
    }

    @Test
    fun `extractText concatenates multiple text parts`() {
        val parts = listOf(
            buildJsonObject { put("kind", "text"); put("text", "Hello") },
            buildJsonObject { put("kind", "text"); put("text", " world") },
        )
        assertEquals("Hello world", extractText(parts))
    }

    @Test
    fun `extractText returns empty string for empty parts list`() {
        assertEquals("", extractText(emptyList()))
    }

    @Test
    fun `extractText returns empty string for text part with missing text field`() {
        val parts = listOf(buildJsonObject { put("kind", "text") })
        assertEquals("", extractText(parts))
    }

    @Test
    fun `extractText returns raw JSON for unknown part type`() {
        val parts = listOf(buildJsonObject { put("kind", "unknown"); put("data", "x") })
        val result = extractText(parts)
        assertTrue(result.isNotEmpty())
    }

    @Test
    fun `extractText handles mixed part types`() {
        val parts = listOf(
            buildJsonObject { put("kind", "text"); put("text", "Answer: ") },
            buildJsonObject { put("kind", "data"); put("data", buildJsonObject { put("value", 42) }) },
        )
        val result = extractText(parts)
        assertTrue(result.startsWith("Answer: "))
    }

    @Test
    fun `extractText handles non-object JSON element gracefully`() {
        val parts = buildJsonArray {
            add(buildJsonObject { put("kind", "text"); put("text", "ok") })
        }.toList()
        assertEquals("ok", extractText(parts))
    }
}
