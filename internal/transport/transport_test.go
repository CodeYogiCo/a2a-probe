package transport

import (
	"encoding/json"
	"testing"
)

func TestParseResponseOK(t *testing.T) {
	body := []byte(`{"jsonrpc":"2.0","id":"1","result":{"state":"completed"}}`)
	result, err := parseResponse(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(result, &obj); err != nil {
		t.Fatalf("result is not valid JSON object: %v", err)
	}
	var state string
	json.Unmarshal(obj["state"], &state)
	if state != "completed" {
		t.Errorf("state: want completed, got %s", state)
	}
}

func TestParseResponseNullResult(t *testing.T) {
	body := []byte(`{"jsonrpc":"2.0","id":"1","result":null}`)
	result, err := parseResponse(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(result) != "null" {
		t.Errorf("want null, got %s", result)
	}
}

func TestParseResponseMissingResult(t *testing.T) {
	body := []byte(`{"jsonrpc":"2.0","id":"1"}`)
	result, err := parseResponse(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(result) != "null" {
		t.Errorf("want null for missing result, got %s", result)
	}
}

func TestParseResponseRPCError(t *testing.T) {
	body := []byte(`{"jsonrpc":"2.0","id":"1","error":{"code":-32601,"message":"Method not found"}}`)
	_, err := parseResponse(body)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	rpcErr, ok := err.(*RPCError)
	if !ok {
		t.Fatalf("expected *RPCError, got %T", err)
	}
	if rpcErr.Code != -32601 {
		t.Errorf("code: want -32601, got %d", rpcErr.Code)
	}
	if rpcErr.Message != "Method not found" {
		t.Errorf("message: want 'Method not found', got %s", rpcErr.Message)
	}
}

func TestParseResponseInvalidJSON(t *testing.T) {
	_, err := parseResponse([]byte(`not-json`))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestRPCErrorMessage(t *testing.T) {
	e := &RPCError{Code: -32000, Message: "server error"}
	want := "JSON-RPC error -32000: server error"
	if e.Error() != want {
		t.Errorf("Error(): want %q, got %q", want, e.Error())
	}
}

func TestCheckRPCErrorNoError(t *testing.T) {
	obj := map[string]json.RawMessage{
		"result": json.RawMessage(`"ok"`),
	}
	if err := checkRPCError(obj); err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestCheckRPCErrorWithError(t *testing.T) {
	obj := map[string]json.RawMessage{
		"error": json.RawMessage(`{"code":-32700,"message":"Parse error"}`),
	}
	err := checkRPCError(obj)
	if err == nil {
		t.Fatal("expected error")
	}
	rpcErr := err.(*RPCError)
	if rpcErr.Code != -32700 {
		t.Errorf("code: want -32700, got %d", rpcErr.Code)
	}
}

func TestExtractResult(t *testing.T) {
	obj := map[string]json.RawMessage{
		"result": json.RawMessage(`42`),
	}
	r := extractResult(obj)
	if string(r) != "42" {
		t.Errorf("want 42, got %s", r)
	}
}

func TestExtractResultMissing(t *testing.T) {
	obj := map[string]json.RawMessage{}
	r := extractResult(obj)
	if string(r) != "null" {
		t.Errorf("want null, got %s", r)
	}
}
