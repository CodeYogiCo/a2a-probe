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

// ── parseSendResult ───────────────────────────────────────────────────────────

func TestParseSendResultMessage(t *testing.T) {
	raw := json.RawMessage(`{"kind":"message","role":"agent","parts":[{"kind":"text","text":"hi"}]}`)
	res, err := parseSendResult(raw)
	if err != nil {
		t.Fatal(err)
	}
	if res.Task != nil || res.Message == nil {
		t.Fatalf("want Message, got %+v", res)
	}
	if ExtractText(res.Message.Parts) != "hi" {
		t.Errorf("text: want 'hi', got %q", ExtractText(res.Message.Parts))
	}
}

func TestParseSendResultTaskByKind(t *testing.T) {
	// Agent answers message/send with a Task carrying the content in artifacts.
	raw := json.RawMessage(`{"kind":"task","id":"t9","status":{"state":"completed"},"artifacts":[{"index":0,"parts":[{"kind":"text","text":"red dress"}]}]}`)
	res, err := parseSendResult(raw)
	if err != nil {
		t.Fatal(err)
	}
	if res.Message != nil || res.Task == nil {
		t.Fatalf("want Task, got %+v", res)
	}
	if res.Task.ID != "t9" {
		t.Errorf("id: want t9, got %s", res.Task.ID)
	}
	if len(res.Task.Artifacts) != 1 {
		t.Fatalf("artifacts: want 1, got %d", len(res.Task.Artifacts))
	}
}

func TestParseSendResultTaskWithoutKind(t *testing.T) {
	// Some agents omit "kind"; a status object with no parts is still a Task.
	raw := json.RawMessage(`{"id":"t10","status":{"state":"working"}}`)
	res, err := parseSendResult(raw)
	if err != nil {
		t.Fatal(err)
	}
	if res.Task == nil {
		t.Fatalf("want Task via structural detection, got %+v", res)
	}
	if res.Task.Status.State != model.TaskStateWorking {
		t.Errorf("state: want working, got %s", res.Task.Status.State)
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

func TestCoerceStreamEventUnwrapsResultEnvelope(t *testing.T) {
	// Many agents wrap each SSE payload in a JSON-RPC envelope.
	raw := json.RawMessage(`{"jsonrpc":"2.0","id":"1","result":{"id":"t1","status":{"state":"working"},"final":false}}`)
	ev := coerceStreamEvent(raw)
	if ev.Status == nil {
		t.Fatalf("expected Status after unwrapping result envelope, got %+v", ev)
	}
	if ev.Status.Status.State != model.TaskStateWorking {
		t.Errorf("state: want working, got %s", ev.Status.Status.State)
	}
}

func TestCoerceStreamEventTaskSnapshot(t *testing.T) {
	// A full Task delivered as a stream event (kind=task / has artifacts).
	raw := json.RawMessage(`{"kind":"task","id":"t1","status":{"state":"completed"},"artifacts":[{"index":0,"parts":[{"kind":"text","text":"done"}]}]}`)
	ev := coerceStreamEvent(raw)
	if ev.Task == nil {
		t.Fatalf("expected Task event, got %+v", ev)
	}
	if len(ev.Task.Artifacts) != 1 {
		t.Errorf("artifacts: want 1, got %d", len(ev.Task.Artifacts))
	}
}

func TestCoerceStreamEventTaskInResultEnvelope(t *testing.T) {
	// The regression: a Task wrapped in a result envelope (what returned nothing).
	raw := json.RawMessage(`{"jsonrpc":"2.0","id":"1","result":{"kind":"task","id":"t1","status":{"state":"completed"},"artifacts":[{"index":0,"parts":[{"kind":"text","text":"red dress"}]}]}}`)
	ev := coerceStreamEvent(raw)
	if ev.Task == nil {
		t.Fatalf("expected Task after unwrapping result envelope, got %+v", ev)
	}
}

func TestCardBasesOriginFirst(t *testing.T) {
	// A service endpoint with a path must still resolve the card at the origin.
	got := cardBases("https://host.example.com/a2a")
	if len(got) != 2 || got[0] != "https://host.example.com" || got[1] != "https://host.example.com/a2a" {
		t.Errorf("want [origin, full], got %v", got)
	}
	// A bare origin yields just the origin.
	got = cardBases("https://host.example.com/")
	if len(got) != 1 || got[0] != "https://host.example.com" {
		t.Errorf("want [origin], got %v", got)
	}
}

func TestPartBuilders(t *testing.T) {
	var p map[string]any

	json.Unmarshal(DataPart(json.RawMessage(`{"q":1}`)), &p)
	if p["kind"] != "data" {
		t.Errorf("data part kind: got %v", p["kind"])
	}

	json.Unmarshal(FilePartURI("https://x/y.pdf", "y.pdf", "application/pdf"), &p)
	if p["kind"] != "file" {
		t.Errorf("file part kind: got %v", p["kind"])
	}
	f := p["file"].(map[string]any)
	if f["uri"] != "https://x/y.pdf" || f["name"] != "y.pdf" {
		t.Errorf("file uri part wrong: %v", f)
	}

	json.Unmarshal(FilePartBytes([]byte("hi"), "a.txt", "text/plain"), &p)
	f = p["file"].(map[string]any)
	if f["bytes"] != "aGk=" { // base64("hi")
		t.Errorf("file bytes wrong: %v", f["bytes"])
	}
}

func TestMakeMessageMultiPart(t *testing.T) {
	m := MakeMessage("m1", []json.RawMessage{TextPart("hi"), DataPart(json.RawMessage(`{"a":1}`))}, json.RawMessage(`{"x":true}`))
	if len(m.Parts) != 2 {
		t.Fatalf("parts: want 2, got %d", len(m.Parts))
	}
	if string(m.Metadata) != `{"x":true}` {
		t.Errorf("metadata: got %s", m.Metadata)
	}
}
