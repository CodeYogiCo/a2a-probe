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

// SSETransport sends JSON-RPC over HTTP and reads streaming responses as SSE.
type SSETransport struct {
	rpcEndpoint string
	sseEndpoint string
	client      *http.Client
	pendingBody string
}

// NewSSE creates an SSETransport. If sseEndpoint is empty it is derived from rpcEndpoint.
func NewSSE(rpcEndpoint, sseEndpoint string) *SSETransport {
	if sseEndpoint == "" {
		sseEndpoint = strings.TrimRight(strings.TrimSuffix(strings.TrimRight(rpcEndpoint, "/"), "/rpc"), "/")
	}
	return &SSETransport{
		rpcEndpoint: strings.TrimRight(rpcEndpoint, "/"),
		sseEndpoint: sseEndpoint,
		client: &http.Client{
			Timeout: 90 * time.Second,
		},
	}
}

func (t *SSETransport) Call(method string, params json.RawMessage) (json.RawMessage, error) {
	envelope := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  method,
		"params":  params,
		"id":      uuid.New().String(),
	}
	body, _ := json.Marshal(envelope)

	var targetURL string
	if method == "tasks/sendSubscribe" || method == "tasks/resubscribe" {
		targetURL = t.sseEndpoint
	} else {
		targetURL = t.rpcEndpoint
	}

	req, err := http.NewRequest(http.MethodPost, targetURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

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
		t.pendingBody = string(respBody)
		for _, line := range strings.Split(t.pendingBody, "\n") {
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

func (t *SSETransport) Stream() <-chan json.RawMessage {
	ch := make(chan json.RawMessage, 256)
	go func() {
		defer close(ch)

		if t.pendingBody != "" {
			body := t.pendingBody
			t.pendingBody = ""
			skippedFirst := false
			for _, line := range strings.Split(body, "\n") {
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
				ch <- json.RawMessage(text)
			}
			return
		}

		// Fallback: GET the SSE endpoint directly
		resp, err := t.client.Get(t.sseEndpoint)
		if err != nil {
			return
		}
		defer resp.Body.Close()
		respBody, _ := io.ReadAll(resp.Body)
		for _, line := range strings.Split(string(respBody), "\n") {
			if !strings.HasPrefix(line, "data:") {
				continue
			}
			text := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if text == "" || text == "[DONE]" {
				continue
			}
			ch <- json.RawMessage(text)
		}
	}()
	return ch
}

func (t *SSETransport) Close() error { return nil }
