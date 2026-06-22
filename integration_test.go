package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// binPath is the compiled CLI built once for the whole package.
var binPath string

// TestMain builds the a2a-probe binary so the integration tests can exercise
// the real command end-to-end against a fake A2A server.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "a2a-probe-it")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(dir)

	binPath = filepath.Join(dir, "a2a-probe")
	build := exec.Command("go", "build", "-o", binPath, ".")
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		panic("failed to build binary: " + err.Error())
	}

	os.Exit(m.Run())
}

// fakeA2AServer is a minimal A2A agent: it inspects the JSON-RPC method and
// replies with either a JSON Task (message/send) or an SSE stream
// (message/stream).
func fakeA2AServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var env struct {
			ID     string          `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		_ = json.Unmarshal(body, &env)

		switch env.Method {
		case "message/send":
			// Answer with a Task carrying the content in artifacts — the shape
			// that previously rendered as an empty "Agent:" line.
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"jsonrpc":"2.0","id":"`+env.ID+`","result":{`+
				`"kind":"task","id":"task-42","status":{"state":"completed"},`+
				`"artifacts":[{"index":0,"parts":[{"kind":"text","text":"red dress, blue dress"}]}]}}`)

		case "message/stream":
			w.Header().Set("Content-Type", "text/event-stream")
			// First data line is the JSON-RPC envelope (consumed by Call);
			// the rest are streamed events.
			io.WriteString(w, "data: {\"jsonrpc\":\"2.0\",\"id\":\""+env.ID+"\",\"result\":{}}\n\n")
			io.WriteString(w, "data: {\"id\":\"task-7\",\"status\":{\"state\":\"working\"},\"final\":false}\n\n")
			io.WriteString(w, "data: {\"id\":\"task-7\",\"artifact\":{\"index\":0,\"parts\":[{\"kind\":\"text\",\"text\":\"streamed answer\"}]}}\n\n")
			io.WriteString(w, "data: {\"id\":\"task-7\",\"status\":{\"state\":\"completed\"},\"final\":true}\n\n")

		default:
			http.Error(w, "unexpected method: "+env.Method, http.StatusBadRequest)
		}
	}))
}

// run executes the built CLI and returns combined stdout+stderr.
func run(t *testing.T, args ...string) string {
	t.Helper()
	out, err := exec.Command(binPath, args...).CombinedOutput()
	if err != nil {
		t.Fatalf("a2a-probe %s failed: %v\noutput:\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

// TestSendTaskOverHTTP verifies that a Task response to message/send is
// rendered with its artifact content (regression test for the empty "Agent:"
// bug).
func TestSendTaskOverHTTP(t *testing.T) {
	srv := fakeA2AServer(t)
	defer srv.Close()

	out := run(t, "-s", srv.URL, "send", "find me dresses")
	if !strings.Contains(out, "red dress, blue dress") {
		t.Errorf("expected artifact text in output, got:\n%s", out)
	}
}

// TestSendStreamOverSSE verifies streaming results come through the SSE
// transport end-to-end.
func TestSendStreamOverSSE(t *testing.T) {
	srv := fakeA2AServer(t)
	defer srv.Close()

	out := run(t, "-s", srv.URL, "-t", "sse", "send", "--stream", "hello")
	if !strings.Contains(out, "streamed answer") {
		t.Errorf("expected streamed artifact text in output, got:\n%s", out)
	}
	if !strings.Contains(out, "completed") {
		t.Errorf("expected final 'completed' status in output, got:\n%s", out)
	}
}
