package transport

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/codeyogico/a2a-probe/internal/debug"
	"github.com/google/uuid"
)

// SSETransport sends JSON-RPC over HTTP and reads streaming responses as SSE.
type SSETransport struct {
	rpcEndpoint string
	sseEndpoint string
	client      *http.Client
	// Set by a streaming Call; consumed incrementally by Stream.
	streamBody    io.ReadCloser
	streamScanner *bufio.Scanner
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

	isStreaming := strings.HasSuffix(method, "/stream") ||
		strings.HasSuffix(method, "Subscribe") ||
		strings.HasSuffix(method, "/resubscribe")

	targetURL := t.rpcEndpoint
	if method == "tasks/sendSubscribe" || method == "tasks/resubscribe" {
		targetURL = t.sseEndpoint
	}

	req, err := http.NewRequest(http.MethodPost, targetURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if isStreaming {
		req.Header.Set("Accept", "text/event-stream")
	}

	debug.Logf("→ SSE POST %s  method=%s", targetURL, method)
	debug.Logf("  request: %s", debug.Truncate(string(body), 2000))

	resp, err := t.client.Do(req)
	if err != nil {
		debug.Logf("  transport error: %v", err)
		return nil, err
	}

	ct := resp.Header.Get("Content-Type")
	debug.Logf("← HTTP %d  Content-Type=%s", resp.StatusCode, ct)

	if strings.Contains(ct, "text/event-stream") {
		scanner := newSSEScanner(resp.Body)
		if isStreaming {
			// Keep the body open; Stream() emits every event (including the
			// first) as it arrives.
			t.streamBody = resp.Body
			t.streamScanner = scanner
			return json.RawMessage("null"), nil
		}
		first, ok := nextSSEData(scanner)
		resp.Body.Close()
		if !ok {
			return nil, &RPCError{Code: -32000, Message: "empty SSE stream"}
		}
		debug.Logf("  event: %s", debug.Truncate(first, 2000))
		return parseResponse([]byte(first))
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	debug.Logf("  response: %s", debug.Truncate(string(respBody), 4000))
	if strings.Contains(ct, "application/json") {
		return parseResponse(respBody)
	}
	return nil, fmt.Errorf("unsupported Content-Type: %s", ct)
}

func (t *SSETransport) Stream() <-chan json.RawMessage {
	scanner := t.streamScanner
	body := t.streamBody
	t.streamScanner, t.streamBody = nil, nil

	ch := make(chan json.RawMessage, 256)
	go func() {
		defer close(ch)
		if body != nil {
			defer body.Close()
		}

		if scanner != nil {
			for {
				data, ok := nextSSEData(scanner)
				if !ok {
					return
				}
				debug.Logf("  event: %s", debug.Truncate(data, 2000))
				ch <- json.RawMessage(data)
			}
		}

		// Fallback: no pending stream (e.g. resubscribe via GET).
		resp, err := t.client.Get(t.sseEndpoint)
		if err != nil {
			return
		}
		defer resp.Body.Close()
		sc := newSSEScanner(resp.Body)
		for {
			data, ok := nextSSEData(sc)
			if !ok {
				return
			}
			ch <- json.RawMessage(data)
		}
	}()
	return ch
}

func (t *SSETransport) Close() error { return nil }
