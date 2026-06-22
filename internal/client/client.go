package client

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/codeyogico/a2a-probe/internal/model"
	"github.com/codeyogico/a2a-probe/internal/transport"
)

// StreamEvent is a discriminated union of streaming response types.
type StreamEvent struct {
	Status   *model.TaskStatusUpdateEvent
	Artifact *model.TaskArtifactUpdateEvent
	Message  *model.Message
	Raw      json.RawMessage
}

// A2AClient is a transport-agnostic high-level A2A client.
type A2AClient struct {
	t transport.Transport
}

// New wraps any transport in an A2AClient.
func New(t transport.Transport) *A2AClient {
	return &A2AClient{t: t}
}

// Close releases the underlying transport.
func (c *A2AClient) Close() error {
	return c.t.Close()
}

func marshal(v interface{}) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}

// SendResult is the result of a message/send call. Per the A2A spec the agent
// may answer with either a Message or a Task, so exactly one field is set.
type SendResult struct {
	Message *model.Message
	Task    *model.Task
}

// SendMessage implements the message/send RPC (A2A ≥0.4).
func (c *A2AClient) SendMessage(msg model.Message) (*SendResult, error) {
	params := map[string]interface{}{"message": msg}
	raw, err := c.t.Call("message/send", marshal(params))
	if err != nil {
		return nil, err
	}
	return parseSendResult(raw)
}

// parseSendResult decodes a message/send result, which is either a Message or a
// Task. It discriminates on "kind"; some agents omit it, so it falls back to
// structural detection: a "status" object with no "parts" is a Task.
func parseSendResult(raw json.RawMessage) (*SendResult, error) {
	var probe struct {
		Kind   string          `json:"kind"`
		Status json.RawMessage `json:"status"`
		Parts  json.RawMessage `json:"parts"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		return nil, err
	}
	if probe.Kind == "task" || (probe.Kind == "" && probe.Status != nil && probe.Parts == nil) {
		var t model.Task
		if err := json.Unmarshal(raw, &t); err != nil {
			return nil, err
		}
		return &SendResult{Task: &t}, nil
	}
	var m model.Message
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	return &SendResult{Message: &m}, nil
}

// StreamMessage implements the message/stream RPC (A2A ≥0.4).
// Returns a channel; the caller must drain it.
func (c *A2AClient) StreamMessage(msg model.Message) (<-chan StreamEvent, error) {
	params := map[string]interface{}{"message": msg}
	_, err := c.t.Call("message/stream", marshal(params))
	if err != nil {
		return nil, err
	}
	return c.drainStream(), nil
}

// SendTask implements tasks/send (A2A 0.3 legacy).
func (c *A2AClient) SendTask(params model.TaskSendParams) (*model.Task, error) {
	raw, err := c.t.Call("tasks/send", marshal(params))
	if err != nil {
		return nil, err
	}
	var task model.Task
	if err := json.Unmarshal(raw, &task); err != nil {
		return nil, err
	}
	return &task, nil
}

// GetTask implements tasks/get.
func (c *A2AClient) GetTask(params model.TaskQueryParams) (*model.Task, error) {
	raw, err := c.t.Call("tasks/get", marshal(params))
	if err != nil {
		return nil, err
	}
	var task model.Task
	if err := json.Unmarshal(raw, &task); err != nil {
		return nil, err
	}
	return &task, nil
}

// CancelTask implements tasks/cancel.
func (c *A2AClient) CancelTask(params model.TaskIDParams) error {
	_, err := c.t.Call("tasks/cancel", marshal(params))
	return err
}

// SendSubscribe implements tasks/sendSubscribe and returns a stream.
func (c *A2AClient) SendSubscribe(params model.TaskSendParams) (<-chan StreamEvent, error) {
	_, err := c.t.Call("tasks/sendSubscribe", marshal(params))
	if err != nil {
		return nil, err
	}
	return c.drainStream(), nil
}

// Resubscribe implements tasks/resubscribe and returns a stream.
func (c *A2AClient) Resubscribe(params model.TaskQueryParams) (<-chan StreamEvent, error) {
	_, err := c.t.Call("tasks/resubscribe", marshal(params))
	if err != nil {
		return nil, err
	}
	return c.drainStream(), nil
}

// FetchAgentCard retrieves /.well-known/agent.json from the base URL.
func (c *A2AClient) FetchAgentCard(baseURL string) *model.AgentCard {
	url := strings.TrimRight(baseURL, "/") + "/.well-known/agent.json"
	httpClient := &http.Client{Timeout: 10 * time.Second}
	resp, err := httpClient.Get(url)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil
	}
	var card model.AgentCard
	if err := json.Unmarshal(body, &card); err != nil {
		return nil
	}
	return &card
}

func (c *A2AClient) drainStream() <-chan StreamEvent {
	out := make(chan StreamEvent, 256)
	go func() {
		defer close(out)
		for raw := range c.t.Stream() {
			out <- coerceStreamEvent(raw)
		}
	}()
	return out
}

func coerceStreamEvent(raw json.RawMessage) StreamEvent {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return StreamEvent{Raw: raw}
	}

	// Unwrap tasks/event envelope: {"method":"tasks/event","params":{…}}
	if methodRaw, ok := obj["method"]; ok {
		var method string
		if json.Unmarshal(methodRaw, &method) == nil && method == "tasks/event" {
			if paramsRaw, ok := obj["params"]; ok {
				var inner map[string]json.RawMessage
				if json.Unmarshal(paramsRaw, &inner) == nil {
					obj = inner
					raw = paramsRaw
				}
			}
		}
	}

	if _, hasStatus := obj["status"]; hasStatus {
		var ev model.TaskStatusUpdateEvent
		if json.Unmarshal(raw, &ev) == nil {
			return StreamEvent{Status: &ev}
		}
	}

	if _, hasArtifact := obj["artifact"]; hasArtifact {
		var ev model.TaskArtifactUpdateEvent
		if json.Unmarshal(raw, &ev) == nil {
			return StreamEvent{Artifact: &ev}
		}
	}

	// Plain message (kind="message" or has role+parts)
	if kindRaw, ok := obj["kind"]; ok {
		var kind string
		if json.Unmarshal(kindRaw, &kind) == nil && kind == "message" {
			var msg model.Message
			if json.Unmarshal(raw, &msg) == nil {
				return StreamEvent{Message: &msg}
			}
		}
	}
	if _, hasRole := obj["role"]; hasRole {
		if _, hasParts := obj["parts"]; hasParts {
			var msg model.Message
			if json.Unmarshal(raw, &msg) == nil {
				return StreamEvent{Message: &msg}
			}
		}
	}

	return StreamEvent{Raw: raw}
}

// BuildClient creates an A2AClient from the global CLI options.
func BuildClient(server, transportName string) (*A2AClient, error) {
	var t transport.Transport
	switch transportName {
	case "sse":
		t = transport.NewSSE(server, "")
	case "ws":
		t = transport.NewWebSocket(server)
	case "stdio":
		t = transport.NewStdio()
	default:
		t = transport.NewHTTP(server)
	}
	return New(t), nil
}

// MakeTextMessage constructs a user Message with a single text part.
func MakeTextMessage(text, messageID string) model.Message {
	part, _ := json.Marshal(map[string]string{"kind": "text", "text": text})
	return model.Message{
		Kind:      "message",
		Role:      model.RoleUser,
		Parts:     []json.RawMessage{part},
		MessageID: messageID,
	}
}

// PartText extracts plain text from a message part, returning "" for non-text parts.
func PartText(raw json.RawMessage) string {
	var obj map[string]json.RawMessage
	if json.Unmarshal(raw, &obj) != nil {
		return fmt.Sprintf("%s", raw)
	}
	kindRaw, okKind := obj["kind"]
	typeRaw, okType := obj["type"]
	var kind string
	if okKind {
		json.Unmarshal(kindRaw, &kind)
	} else if okType {
		json.Unmarshal(typeRaw, &kind)
	}
	if kind == "text" {
		var text string
		if textRaw, ok := obj["text"]; ok {
			json.Unmarshal(textRaw, &text)
		}
		return text
	}
	return fmt.Sprintf("%s", raw)
}

// ExtractText joins all text parts in a message.
func ExtractText(parts []json.RawMessage) string {
	var sb strings.Builder
	for _, p := range parts {
		sb.WriteString(PartText(p))
	}
	return sb.String()
}
