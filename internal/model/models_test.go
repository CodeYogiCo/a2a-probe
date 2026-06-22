package model

import (
	"encoding/json"
	"testing"
)

func TestTaskStateRoundtrip(t *testing.T) {
	cases := []TaskState{
		TaskStateSubmitted, TaskStateWorking, TaskStateCompleted,
		TaskStateFailed, TaskStateCanceled, TaskStateUnknown,
	}
	for _, s := range cases {
		b, err := json.Marshal(s)
		if err != nil {
			t.Fatalf("marshal %s: %v", s, err)
		}
		var got TaskState
		if err := json.Unmarshal(b, &got); err != nil {
			t.Fatalf("unmarshal %s: %v", s, err)
		}
		if got != s {
			t.Errorf("roundtrip: want %s, got %s", s, got)
		}
	}
}

func TestRoleRoundtrip(t *testing.T) {
	for _, r := range []Role{RoleUser, RoleAssistant, RoleAgent} {
		b, _ := json.Marshal(r)
		var got Role
		json.Unmarshal(b, &got)
		if got != r {
			t.Errorf("role roundtrip: want %s, got %s", r, got)
		}
	}
}

func TestMessageMarshal(t *testing.T) {
	part, _ := json.Marshal(map[string]string{"kind": "text", "text": "hello"})
	msg := Message{
		Kind:      "message",
		Role:      RoleUser,
		Parts:     []json.RawMessage{part},
		MessageID: "msg-1",
	}
	b, err := json.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}
	var got Message
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got.Role != RoleUser {
		t.Errorf("role: want user, got %s", got.Role)
	}
	if got.MessageID != "msg-1" {
		t.Errorf("messageId: want msg-1, got %s", got.MessageID)
	}
	if len(got.Parts) != 1 {
		t.Errorf("parts: want 1, got %d", len(got.Parts))
	}
}

func TestTaskMarshal(t *testing.T) {
	sid := "sess-1"
	task := Task{
		ID:        "task-1",
		SessionID: &sid,
		Status:    TaskStatus{State: TaskStateCompleted},
	}
	b, err := json.Marshal(task)
	if err != nil {
		t.Fatal(err)
	}
	var got Task
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got.ID != "task-1" {
		t.Errorf("id: want task-1, got %s", got.ID)
	}
	if got.Status.State != TaskStateCompleted {
		t.Errorf("state: want completed, got %s", got.Status.State)
	}
	if got.SessionID == nil || *got.SessionID != "sess-1" {
		t.Errorf("sessionId: want sess-1")
	}
}

func TestCliConfigRoundtrip(t *testing.T) {
	cfg := CliConfig{
		Servers: map[string]ServerConfig{
			"local": {URL: "http://localhost:8000", Transport: "http"},
			"prod":  {URL: "https://agent.example.com", Transport: "sse"},
		},
	}
	b, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var got CliConfig
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Servers) != 2 {
		t.Errorf("servers: want 2, got %d", len(got.Servers))
	}
	if got.Servers["prod"].URL != "https://agent.example.com" {
		t.Errorf("prod url mismatch")
	}
}

func TestAgentCardOptionalFields(t *testing.T) {
	raw := `{"name":"MyAgent","capabilities":{"streaming":true}}`
	var card AgentCard
	if err := json.Unmarshal([]byte(raw), &card); err != nil {
		t.Fatal(err)
	}
	if card.Name == nil || *card.Name != "MyAgent" {
		t.Errorf("name: want MyAgent")
	}
	if card.Capabilities == nil || card.Capabilities.Streaming == nil || !*card.Capabilities.Streaming {
		t.Errorf("streaming capability expected true")
	}
	if card.Version != nil {
		t.Errorf("version should be nil")
	}
}

func TestJsonRpcResponseParsing(t *testing.T) {
	raw := `{"jsonrpc":"2.0","id":"1","result":{"id":"task-42","status":{"state":"completed"}}}`
	var resp JsonRpcResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.JSONRPC != "2.0" {
		t.Errorf("jsonrpc: want 2.0")
	}
	if resp.Error != nil {
		t.Errorf("unexpected error field")
	}
	if resp.Result == nil {
		t.Errorf("result should not be nil")
	}
}

func TestJsonRpcErrorParsing(t *testing.T) {
	raw := `{"jsonrpc":"2.0","id":"1","error":{"code":-32601,"message":"Method not found"}}`
	var resp JsonRpcResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Error == nil {
		t.Fatal("expected error field")
	}
	if resp.Error.Code != -32601 {
		t.Errorf("code: want -32601, got %d", resp.Error.Code)
	}
	if resp.Error.Message != "Method not found" {
		t.Errorf("message: want 'Method not found', got %s", resp.Error.Message)
	}
}
