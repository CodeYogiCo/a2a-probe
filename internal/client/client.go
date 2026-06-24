package client

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/codeyogico/a2a-probe/internal/debug"
	"github.com/codeyogico/a2a-probe/internal/model"
	"github.com/codeyogico/a2a-probe/internal/transport"
)

// StreamEvent is a discriminated union of streaming response types.
type StreamEvent struct {
	Task     *model.Task
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

// agentCardPaths are the well-known locations to probe, in order. The original
// path and the newer spec name are both tried for compatibility.
var agentCardPaths = []string{"/.well-known/agent.json", "/.well-known/agent-card.json"}

// cardBases returns the base URLs to probe for an agent card. Well-known URIs
// are rooted at the origin (RFC 8615), so the origin is tried first; the full
// path is kept as a fallback for agents that (non-compliantly) serve the card
// under their service path.
func cardBases(raw string) []string {
	full := strings.TrimRight(raw, "/")
	if u, err := url.Parse(raw); err == nil && u.Scheme != "" && u.Host != "" {
		origin := u.Scheme + "://" + u.Host
		if full != origin {
			return []string{origin, full}
		}
		return []string{origin}
	}
	return []string{full}
}

// FetchAgentCardRaw fetches the raw agent card JSON, trying each base/well-known
// combination. It returns the bytes and the URL it was found at.
func (c *A2AClient) FetchAgentCardRaw(baseURL string) (json.RawMessage, string, error) {
	httpClient := &http.Client{Timeout: 10 * time.Second}
	var lastErr error
	for _, base := range cardBases(baseURL) {
		for _, p := range agentCardPaths {
			cardURL := base + p
			debug.Logf("→ GET agent card %s", cardURL)
			resp, err := httpClient.Get(cardURL)
			if err != nil {
				lastErr = err
				continue
			}
			body, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			debug.Logf("← agent card %s: HTTP %d (%d bytes)", cardURL, resp.StatusCode, len(body))
			if err != nil {
				lastErr = err
				continue
			}
			if resp.StatusCode != http.StatusOK {
				lastErr = fmt.Errorf("HTTP %d from %s", resp.StatusCode, cardURL)
				continue
			}
			if !json.Valid(body) {
				lastErr = fmt.Errorf("invalid JSON from %s", cardURL)
				continue
			}
			return json.RawMessage(body), cardURL, nil
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no agent card found at %s", baseURL)
	}
	return nil, "", lastErr
}

func (c *A2AClient) FetchAgentCard(baseURL string) *model.AgentCard {
	raw, _, err := c.FetchAgentCardRaw(baseURL)
	if err != nil {
		return nil
	}
	var card model.AgentCard
	if json.Unmarshal(raw, &card) != nil {
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

	// Unwrap a JSON-RPC envelope: {"jsonrpc":"2.0","id":…,"result":{…}}.
	// SSE events from many agents wrap the real payload under "result".
	if resultRaw, ok := obj["result"]; ok {
		var inner map[string]json.RawMessage
		if json.Unmarshal(resultRaw, &inner) == nil && len(inner) > 0 {
			obj = inner
			raw = resultRaw
		}
	}

	// A full Task snapshot (kind="task" or carries artifacts) — render in full.
	// Checked before status, since a Task also has a status field.
	if isTaskEvent(obj) {
		var t model.Task
		if json.Unmarshal(raw, &t) == nil {
			return StreamEvent{Task: &t}
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

// isTaskEvent reports whether a decoded event object is a full Task snapshot
// rather than an incremental status/artifact update.
func isTaskEvent(obj map[string]json.RawMessage) bool {
	if kindRaw, ok := obj["kind"]; ok {
		var kind string
		if json.Unmarshal(kindRaw, &kind) == nil && kind == "task" {
			return true
		}
	}
	// A plural "artifacts" array is a Task; the singular "artifact" is an update.
	_, hasArtifacts := obj["artifacts"]
	return hasArtifacts
}

// BuildClient creates an A2AClient from the global CLI options.
func BuildClient(server, transportName string) (*A2AClient, error) {
	return BuildClientWithHeaders(server, transportName, nil)
}

// BuildClientWithHeaders is BuildClient with extra HTTP headers (auth, etc.)
// applied to every request the transport makes.
func BuildClientWithHeaders(server, transportName string, headers map[string]string) (*A2AClient, error) {
	var t transport.Transport
	switch transportName {
	case "sse":
		t = transport.NewSSE(server, "", headers)
	case "ws":
		t = transport.NewWebSocket(server, headers)
	case "stdio":
		t = transport.NewStdio()
	default:
		t = transport.NewHTTP(server, headers)
	}
	return New(t), nil
}

// MakeTextMessage constructs a user Message with a single text part.
func MakeTextMessage(text, messageID string) model.Message {
	return MakeMessage(messageID, []json.RawMessage{TextPart(text)}, nil)
}

// MakeMessage builds a user Message from pre-built parts and optional metadata.
func MakeMessage(messageID string, parts []json.RawMessage, metadata json.RawMessage) model.Message {
	return model.Message{
		Kind:      "message",
		Role:      model.RoleUser,
		Parts:     parts,
		MessageID: messageID,
		Metadata:  metadata,
	}
}

// TextPart builds a text part.
func TextPart(text string) json.RawMessage {
	b, _ := json.Marshal(map[string]any{"kind": "text", "text": text})
	return b
}

// DataPart builds a structured-data part from raw JSON.
func DataPart(data json.RawMessage) json.RawMessage {
	b, _ := json.Marshal(map[string]any{"kind": "data", "data": data})
	return b
}

// FilePartURI builds a file part referencing a URI.
func FilePartURI(uri, name, mimeType string) json.RawMessage {
	file := map[string]any{"uri": uri}
	if name != "" {
		file["name"] = name
	}
	if mimeType != "" {
		file["mimeType"] = mimeType
	}
	b, _ := json.Marshal(map[string]any{"kind": "file", "file": file})
	return b
}

// FilePartBytes builds a file part with base64-encoded inline bytes.
func FilePartBytes(data []byte, name, mimeType string) json.RawMessage {
	file := map[string]any{"bytes": base64.StdEncoding.EncodeToString(data)}
	if name != "" {
		file["name"] = name
	}
	if mimeType != "" {
		file["mimeType"] = mimeType
	}
	b, _ := json.Marshal(map[string]any{"kind": "file", "file": file})
	return b
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
