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

	"github.com/google/uuid"
)

// HTTPTransport sends JSON-RPC over HTTP (and handles SSE responses).
type HTTPTransport struct {
	endpoint string
	client   *http.Client
	// Set by a streaming Call; consumed incrementally by Stream.
	streamBody    io.ReadCloser
	streamScanner *bufio.Scanner
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

	ct := resp.Header.Get("Content-Type")
	if strings.Contains(ct, "text/event-stream") {
		// Read the SSE stream incrementally. The first data payload is returned
		// as the Call result; Stream() reads the rest as it arrives.
		scanner := newSSEScanner(resp.Body)
		first, ok := nextSSEData(scanner)
		if !ok {
			resp.Body.Close()
			return nil, &RPCError{Code: -32000, Message: "empty SSE stream"}
		}
		if isStreaming {
			t.streamBody = resp.Body
			t.streamScanner = scanner
		} else {
			resp.Body.Close()
		}
		return parseResponse([]byte(first))
	}

	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if strings.Contains(ct, "application/json") {
		return parseResponse(respBody)
	}
	return nil, fmt.Errorf("unsupported Content-Type: %s", ct)
}

func (t *HTTPTransport) Stream() <-chan json.RawMessage {
	scanner := t.streamScanner
	body := t.streamBody
	t.streamScanner, t.streamBody = nil, nil

	ch := make(chan json.RawMessage, 256)
	go func() {
		defer close(ch)
		if body != nil {
			defer body.Close()
		}
		if scanner == nil {
			return
		}
		for {
			data, ok := nextSSEData(scanner)
			if !ok {
				return
			}
			ch <- json.RawMessage(data)
		}
	}()
	return ch
}

func (t *HTTPTransport) Close() error { return nil }
