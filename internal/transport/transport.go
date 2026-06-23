package transport

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// newSSEScanner returns a line scanner over an SSE body with a buffer large
// enough for big JSON event payloads (default bufio cap is only 64 KiB).
func newSSEScanner(r io.Reader) *bufio.Scanner {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	return sc
}

// nextSSEData advances the scanner to the next non-empty `data:` payload,
// returning it and true, or ("", false) when the stream ends.
func nextSSEData(sc *bufio.Scanner) (string, bool) {
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" || data == "[DONE]" {
			continue
		}
		return data, true
	}
	return "", false
}

// Transport is the interface all A2A transports implement.
type Transport interface {
	// Call sends a JSON-RPC request and returns the result.
	Call(method string, params json.RawMessage) (json.RawMessage, error)
	// Stream returns a channel of JSON objects received after the last Call.
	Stream() <-chan json.RawMessage
	// Close releases resources.
	Close() error
}

// RPCError represents a JSON-RPC error response.
type RPCError struct {
	Code    int
	Message string
	Data    json.RawMessage
}

func (e *RPCError) Error() string {
	return fmt.Sprintf("JSON-RPC error %d: %s", e.Code, e.Message)
}

// checkRPCError extracts an error from a parsed response object.
func checkRPCError(obj map[string]json.RawMessage) error {
	raw, ok := obj["error"]
	if !ok {
		return nil
	}
	var rpcErr struct {
		Code    int             `json:"code"`
		Message string          `json:"message"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &rpcErr); err != nil {
		return &RPCError{Code: -32000, Message: "unparseable RPC error"}
	}
	return &RPCError{Code: rpcErr.Code, Message: rpcErr.Message, Data: rpcErr.Data}
}

// extractResult returns the "result" field or null from a parsed response.
func extractResult(obj map[string]json.RawMessage) json.RawMessage {
	if r, ok := obj["result"]; ok {
		return r
	}
	return json.RawMessage("null")
}

// parseResponse unmarshals a JSON-RPC response body, checks for errors,
// and returns the result payload.
func parseResponse(body []byte) (json.RawMessage, error) {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(body, &obj); err != nil {
		return nil, fmt.Errorf("invalid JSON-RPC response: %w", err)
	}
	if err := checkRPCError(obj); err != nil {
		return nil, err
	}
	return extractResult(obj), nil
}
