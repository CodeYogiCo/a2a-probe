package transport

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

// HTTPTransport sends JSON-RPC over HTTP (and handles SSE responses).
type HTTPTransport struct {
	endpoint       string
	client         *http.Client
	pendingSseLines []string
	streamCh       chan json.RawMessage
}

// NewHTTP creates an HTTPTransport targeting the given endpoint URL.
func NewHTTP(endpoint string) *HTTPTransport {
	return &HTTPTransport{
		endpoint: strings.TrimRight(endpoint, "/"),
		client: &http.Client{
			Timeout: 90 * time.Second,
			Transport: &http.Transport{
				ResponseHeaderTimeout: 90 * time.Second,
			},
		},
		streamCh: make(chan json.RawMessage, 256),
	}
}

func (t *HTTPTransport) Call(method string, params json.RawMessage) (json.RawMessage, error) {
	envelope := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  method,
		"params":  params,
		"id":      uuid.New().String(),
	}
	body, _ := json.Marshal(envelope)

	isStreaming := strings.HasSuffix(method, "/stream") ||
		strings.HasSuffix(method, "Subscribe") ||
		strings.HasSuffix(method, "/resubscribe")

	req, err := http.NewRequest(http.MethodPost, t.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if isStreaming {
		req.Header.Set("Accept", "text/event-stream")
	}

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	ct := resp.Header.Get("Content-Type")
	switch {
	case strings.Contains(ct, "application/json"):
		return parseResponse(respBody)

	case strings.Contains(ct, "text/event-stream"):
		lines := strings.Split(string(respBody), "\n")
		t.pendingSseLines = lines

		// Find first data line and return its result
		for _, line := range lines {
			if !strings.HasPrefix(line, "data:") {
				continue
			}
			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if data == "" || data == "[DONE]" {
				continue
			}
			return parseResponse([]byte(data))
		}
		return nil, &RPCError{Code: -32000, Message: "empty SSE stream"}

	default:
		return nil, fmt.Errorf("unsupported Content-Type: %s", ct)
	}
}

func (t *HTTPTransport) Stream() <-chan json.RawMessage {
	ch := make(chan json.RawMessage, 256)
	go func() {
		defer close(ch)
		lines := t.pendingSseLines
		t.pendingSseLines = nil

		skippedFirst := false
		for _, line := range lines {
			if !strings.HasPrefix(line, "data:") {
				continue
			}
			if !skippedFirst {
				skippedFirst = true
				continue
			}
			text := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if text == "" || text == "[DONE]" {
				continue
			}
			var raw json.RawMessage = json.RawMessage(text)
			ch <- raw
		}
	}()
	return ch
}

func (t *HTTPTransport) Close() error { return nil }
