package client

import (
	"encoding/json"
	"testing"

	"github.com/codeyogico/a2a-probe/internal/model"
)

// ── MakeTextMessage ────────────────────────────────────────────────────────────

func TestMakeTextMessage(t *testing.T) {
	msg := MakeTextMessage("hello world", "msg-1")
	if msg.Role != model.RoleUser {
		t.Errorf("role: want user, got %s", msg.Role)
	}
	if msg.Kind != "message" {
		t.Errorf("kind: want message, got %s", msg.Kind)
	}
	if msg.MessageID != "msg-1" {
		t.Errorf("messageId: want msg-1, got %s", msg.MessageID)
	}
	if len(msg.Parts) != 1 {
		t.Fatalf("parts: want 1, got %d", len(msg.Parts))
	}
	var part map[string]string
	if err := json.Unmarshal(msg.Parts[0], &part); err != nil {
		t.Fatal(err)
	}
	if part["kind"] != "text" {
		t.Errorf("part kind: want text, got %s", part["kind"])
	}
	if part["text"] != "hello world" {
		t.Errorf("part text: want 'hello world', got %s", part["text"])
	}
}

// ── ExtractText ───────────────────────────────────────────────────────────────

func TestExtractTextSingle(t *testing.T) {
	part, _ := json.Marshal(map[string]string{"kind": "text", "text": "hello"})
	got := ExtractText([]json.RawMessage{part})
	if got != "hello" {
		t.Errorf("want 'hello', got %q", got)
	}
}

func TestExtractTextMultiple(t *testing.T) {
	p1, _ := json.Marshal(map[string]string{"kind": "text", "text": "foo"})
	p2, _ := json.Marshal(map[string]string{"kind": "text", "text": "bar"})
	got := ExtractText([]json.RawMessage{p1, p2})
	if got != "foobar" {
		t.Errorf("want 'foobar', got %q", got)
	}
}

func TestExtractTextLegacyTypeField(t *testing.T) {
	part, _ := json.Marshal(map[string]string{"type": "text", "text": "legacy"})
	got := ExtractText([]json.RawMessage{part})
	if got != "legacy" {
		t.Errorf("want 'legacy', got %q", got)
	}
}

func TestExtractTextNonText(t *testing.T) {
	part, _ := json.Marshal(map[string]string{"kind": "file", "uri": "http://example.com/f.pdf"})
	got := ExtractText([]json.RawMessage{part})
	// Non-text parts are rendered as their raw JSON
	if got == "" {
		t.Errorf("non-text part should produce non-empty output")
	}
}

func TestExtractTextEmpty(t *testing.T) {
	got := ExtractText(nil)
	if got != "" {
		t.Errorf("empty parts: want '', got %q", got)
	}
}

// ── PartText ─────────────────────────────────────────────────────────────────

func TestPartTextKindText(t *testing.T) {
	raw, _ := json.Marshal(map[string]string{"kind": "text", "text": "world"})
	got := PartText(raw)
	if got != "world" {
		t.Errorf("want 'world', got %q", got)
	}
}

func TestPartTextNonText(t *testing.T) {
	raw, _ := json.Marshal(map[string]string{"kind": "data"})
	got := PartText(raw)
	if got == "" {
		t.Errorf("non-text: want non-empty output")
	}
}

// ── coerceStreamEvent ─────────────────────────────────────────────────────────

func TestCoerceStreamEventStatus(t *testing.T) {
	raw := json.RawMessage(`{"id":"t1","status":{"state":"working"},"final":false}`)
	ev := coerceStreamEvent(raw)
	if ev.Status == nil {
		t.Fatal("expected Status event")
	}
	if ev.Status.ID != "t1" {
		t.Errorf("id: want t1, got %s", ev.Status.ID)
	}
	if ev.Status.Status.State != model.TaskStateWorking {
		t.Errorf("state: want working, got %s", ev.Status.Status.State)
	}
}

func TestCoerceStreamEventArtifact(t *testing.T) {
	raw := json.RawMessage(`{"id":"t1","artifact":{"index":0,"parts":[]}}`)
	ev := coerceStreamEvent(raw)
	if ev.Artifact == nil {
		t.Fatal("expected Artifact event")
	}
	if ev.Artifact.ID != "t1" {
		t.Errorf("id: want t1, got %s", ev.Artifact.ID)
	}
}

func TestCoerceStreamEventMessage(t *testing.T) {
	raw := json.RawMessage(`{"kind":"message","role":"agent","parts":[]}`)
	ev := coerceStreamEvent(raw)
	if ev.Message == nil {
		t.Fatal("expected Message event")
	}
	if ev.Message.Role != model.RoleAgent {
		t.Errorf("role: want agent, got %s", ev.Message.Role)
	}
}

func TestCoerceStreamEventMessageByRoleParts(t *testing.T) {
	raw := json.RawMessage(`{"role":"user","parts":[]}`)
	ev := coerceStreamEvent(raw)
	if ev.Message == nil {
		t.Fatal("expected Message event via role+parts detection")
	}
}

func TestCoerceStreamEventTasksEventEnvelope(t *testing.T) {
	// Simulate the {"method":"tasks/event","params":{…}} envelope
	raw := json.RawMessage(`{"method":"tasks/event","params":{"id":"t2","status":{"state":"completed"},"final":true}}`)
	ev := coerceStreamEvent(raw)
	if ev.Status == nil {
		t.Fatal("expected Status event after unwrapping tasks/event envelope")
	}
	if ev.Status.ID != "t2" {
		t.Errorf("id: want t2, got %s", ev.Status.ID)
	}
	if !ev.Status.Final {
		t.Errorf("final: want true")
	}
}

func TestCoerceStreamEventUnknown(t *testing.T) {
	raw := json.RawMessage(`{"something":"unknown"}`)
	ev := coerceStreamEvent(raw)
	if ev.Raw == nil {
		t.Fatal("expected Raw for unrecognised event")
	}
}

func TestCoerceStreamEventInvalidJSON(t *testing.T) {
	raw := json.RawMessage(`not-json`)
	ev := coerceStreamEvent(raw)
	if ev.Raw == nil {
		t.Fatal("expected Raw for invalid JSON")
	}
}
