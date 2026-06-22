package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func testServer() *Server {
	webFS := fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte("<html><body>test</body></html>")},
	}
	return New(webFS)
}

func TestServeIndexHTML(t *testing.T) {
	srv := testServer()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("GET /: want 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "test") {
		t.Errorf("body should contain index.html content")
	}
}

func TestAgentCardBadServer(t *testing.T) {
	srv := testServer()
	req := httptest.NewRequest(http.MethodGet, "/api/agent-card?server=unknownalias&transport=http", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	// Should return 400 for an unknown alias that's not a URL
	if w.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", w.Code)
	}
	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp["error"] == "" {
		t.Errorf("expected non-empty error field")
	}
}

func TestSendMethodNotAllowed(t *testing.T) {
	srv := testServer()
	req := httptest.NewRequest(http.MethodGet, "/api/send", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("want 405, got %d", w.Code)
	}
}

func TestSendBadJSON(t *testing.T) {
	srv := testServer()
	req := httptest.NewRequest(http.MethodPost, "/api/send", strings.NewReader("not-json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("want 400 for bad JSON, got %d", w.Code)
	}
}

func TestStreamMethodNotAllowed(t *testing.T) {
	srv := testServer()
	req := httptest.NewRequest(http.MethodGet, "/api/stream", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("want 405, got %d", w.Code)
	}
}

func TestStreamBadServer(t *testing.T) {
	srv := testServer()
	body := `{"server":"unknownalias","transport":"http","message":"hello"}`
	req := httptest.NewRequest(http.MethodPost, "/api/stream", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("want 400 for unknown server alias, got %d", w.Code)
	}
}

func TestCancelMethodNotAllowed(t *testing.T) {
	srv := testServer()
	req := httptest.NewRequest(http.MethodGet, "/api/cancel", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("want 405, got %d", w.Code)
	}
}

func TestCancelBadJSON(t *testing.T) {
	srv := testServer()
	req := httptest.NewRequest(http.MethodPost, "/api/cancel", strings.NewReader("not-json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("want 400 for bad JSON, got %d", w.Code)
	}
}

func TestGetTaskBadServer(t *testing.T) {
	srv := testServer()
	req := httptest.NewRequest(http.MethodGet, "/api/task?server=unknownalias&transport=http&id=abc", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("want 400 for unknown server, got %d", w.Code)
	}
}

func TestSendBadServer(t *testing.T) {
	srv := testServer()
	body := `{"server":"unknownalias","transport":"http","message":"hi"}`
	req := httptest.NewRequest(http.MethodPost, "/api/send", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("want 400 for unknown server alias, got %d", w.Code)
	}
}

func TestWriteJSONResponseFormat(t *testing.T) {
	w := httptest.NewRecorder()
	writeJSON(w, map[string]string{"key": "value"})
	if w.Code != http.StatusOK {
		t.Errorf("want 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("content-type: want application/json, got %s", ct)
	}
	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp["key"] != "value" {
		t.Errorf("want key=value")
	}
}

func TestWriteErrResponseFormat(t *testing.T) {
	w := httptest.NewRecorder()
	writeErr(w, "something went wrong", http.StatusBadRequest)
	if w.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", w.Code)
	}
	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp["error"] != "something went wrong" {
		t.Errorf("error field mismatch: %s", resp["error"])
	}
}

func TestSendWithHTTPServerAgent(t *testing.T) {
	// Spin up a fake A2A agent that returns a valid message/send response
	agent := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"jsonrpc":"2.0","id":"1","result":{"kind":"message","role":"agent","parts":[{"kind":"text","text":"pong"}]}}`))
	}))
	defer agent.Close()

	srv := testServer()
	body, _ := json.Marshal(map[string]interface{}{
		"server":    agent.URL,
		"transport": "http",
		"message":   "ping",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/send", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d — body: %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp["message"] == nil {
		t.Errorf("expected message field in response")
	}
}
