package ui

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestExtractTextSinglePart(t *testing.T) {
	part, _ := json.Marshal(map[string]string{"kind": "text", "text": "hello"})
	got := ExtractText([]json.RawMessage{part})
	if got != "hello" {
		t.Errorf("want 'hello', got %q", got)
	}
}

func TestExtractTextMultipleParts(t *testing.T) {
	p1, _ := json.Marshal(map[string]string{"kind": "text", "text": "foo "})
	p2, _ := json.Marshal(map[string]string{"kind": "text", "text": "bar"})
	got := ExtractText([]json.RawMessage{p1, p2})
	if got != "foo bar" {
		t.Errorf("want 'foo bar', got %q", got)
	}
}

func TestExtractTextLegacyTypeField(t *testing.T) {
	part, _ := json.Marshal(map[string]string{"type": "text", "text": "legacy"})
	got := ExtractText([]json.RawMessage{part})
	if got != "legacy" {
		t.Errorf("want 'legacy', got %q", got)
	}
}

func TestExtractTextNil(t *testing.T) {
	if got := ExtractText(nil); got != "" {
		t.Errorf("nil parts: want '', got %q", got)
	}
}

func TestExtractTextNonTextPart(t *testing.T) {
	part, _ := json.Marshal(map[string]string{"kind": "file", "uri": "https://example.com/file.pdf"})
	got := ExtractText([]json.RawMessage{part})
	// Non-text parts are rendered as raw JSON, so result should be non-empty
	if got == "" {
		t.Errorf("non-text part should produce non-empty string")
	}
}

func TestExtractTextInvalidJSON(t *testing.T) {
	// Invalid JSON is rendered as-is (not panicked on)
	got := ExtractText([]json.RawMessage{json.RawMessage(`not-json`)})
	if got == "" {
		t.Errorf("invalid JSON part should produce non-empty fallback")
	}
}

func TestAnsiHelpers(t *testing.T) {
	// Verify helpers don't panic and wrap with reset sequence
	for name, fn := range map[string]func(string) string{
		"Bold": Bold, "Dim": Dim, "Red": Red, "Green": Green,
		"Yellow": Yellow, "Blue": Blue, "Cyan": Cyan, "White": White,
	} {
		result := fn("test")
		if result == "test" {
			t.Errorf("%s: should have applied escape codes", name)
		}
		if len(result) == 0 {
			t.Errorf("%s: should not be empty", name)
		}
	}
}

func TestPrettyJSON(t *testing.T) {
	got := prettyJSON(json.RawMessage(`{"b":2,"a":[1,2]}`))
	// Indented output spans multiple lines.
	if !strings.Contains(got, "\n") {
		t.Errorf("expected multi-line indented JSON, got %q", got)
	}
	// Still valid JSON after indenting.
	var v map[string]interface{}
	if err := json.Unmarshal([]byte(got), &v); err != nil {
		t.Errorf("pretty output is not valid JSON: %v", err)
	}
}

func TestPrettyJSONInvalidFallback(t *testing.T) {
	got := prettyJSON(json.RawMessage(`not json`))
	if got != "not json" {
		t.Errorf("want passthrough of invalid input, got %q", got)
	}
}
