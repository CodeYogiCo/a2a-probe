package transport

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHTTPTransportCall_JSONResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"jsonrpc":"2.0","id":"1","result":{"id":"task-1","status":{"state":"completed"}}}`))
	}))
	defer srv.Close()

	tr := NewHTTP(srv.URL)
	result, err := tr.Call("tasks/get", json.RawMessage(`{"id":"task-1"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(result, &obj); err != nil {
		t.Fatal(err)
	}
	var id string
	json.Unmarshal(obj["id"], &id)
	if id != "task-1" {
		t.Errorf("id: want task-1, got %s", id)
	}
}

func TestHTTPTransportCall_RPCError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"jsonrpc":"2.0","id":"1","error":{"code":-32601,"message":"Method not found"}}`))
	}))
	defer srv.Close()

	tr := NewHTTP(srv.URL)
	_, err := tr.Call("tasks/unknown", json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error")
	}
	rpcErr, ok := err.(*RPCError)
	if !ok {
		t.Fatalf("want *RPCError, got %T", err)
	}
	if rpcErr.Code != -32601 {
		t.Errorf("code: want -32601, got %d", rpcErr.Code)
	}
}

func TestHTTPTransportCall_SSEResponse(t *testing.T) {
	sseBody := "data: {\"jsonrpc\":\"2.0\",\"id\":\"1\",\"result\":{}}\n\ndata: {\"id\":\"t1\",\"status\":{\"state\":\"working\"},\"final\":false}\n\ndata: {\"id\":\"t1\",\"status\":{\"state\":\"completed\"},\"final\":true}\n\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte(sseBody))
	}))
	defer srv.Close()

	tr := NewHTTP(srv.URL)
	result, err := tr.Call("tasks/sendSubscribe", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// First result should be {} (from the first data line)
	_ = result

	// Drain the stream
	var events []json.RawMessage
	for raw := range tr.Stream() {
		events = append(events, raw)
	}
	if len(events) != 2 {
		t.Errorf("stream events: want 2, got %d", len(events))
	}
}

func TestHTTPTransportCall_UnsupportedContentType(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte("hello"))
	}))
	defer srv.Close()

	tr := NewHTTP(srv.URL)
	_, err := tr.Call("tasks/get", json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error for unsupported content type")
	}
}

func TestHTTPTransportClose(t *testing.T) {
	tr := NewHTTP("http://localhost:9999")
	if err := tr.Close(); err != nil {
		t.Errorf("Close() should not error: %v", err)
	}
}

// TestHTTPTransportStreamIncremental proves the stream is read as events
// arrive, not buffered until the server closes. The server holds the
// connection open after the first event; the client must still receive it.
func TestHTTPTransportStreamIncremental(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fl, ok := w.(http.Flusher)
		if !ok {
			t.Error("ResponseWriter is not a Flusher")
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		io.WriteString(w, "data: {\"jsonrpc\":\"2.0\",\"id\":\"1\",\"result\":{}}\n\n")
		fl.Flush()
		io.WriteString(w, "data: {\"id\":\"t1\",\"status\":{\"state\":\"working\"},\"final\":false}\n\n")
		fl.Flush()
		<-release // hold the connection open
		io.WriteString(w, "data: {\"id\":\"t1\",\"status\":{\"state\":\"completed\"},\"final\":true}\n\n")
		fl.Flush()
	}))
	defer srv.Close()
	defer close(release)

	tr := NewHTTP(srv.URL)
	if _, err := tr.Call("message/stream", json.RawMessage(`{}`)); err != nil {
		t.Fatalf("Call failed: %v", err)
	}
	ch := tr.Stream()

	// The first event must arrive while the server is still holding the
	// connection (before we release it). With buffered ReadAll, Call itself
	// would block here forever.
	select {
	case ev := <-ch:
		if !strings.Contains(string(ev), "working") {
			t.Errorf("first event: want 'working', got %s", ev)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("first event did not arrive incrementally")
	}
}
