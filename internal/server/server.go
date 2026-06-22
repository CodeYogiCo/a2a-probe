package server

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"

	"github.com/codeyogico/a2a-probe/internal/client"
	"github.com/codeyogico/a2a-probe/internal/config"
	"github.com/codeyogico/a2a-probe/internal/model"
	"github.com/google/uuid"
)

// Server serves the web UI and proxies A2A API calls.
type Server struct {
	webFS fs.FS
}

// New creates a Server using webFS as the static file root.
func New(webFS fs.FS) *Server {
	return &Server{webFS: webFS}
}

// Handler returns the HTTP handler for the server.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.FS(s.webFS)))
	mux.HandleFunc("/api/agent-card", s.agentCard)
	mux.HandleFunc("/api/send", s.send)
	mux.HandleFunc("/api/stream", s.stream)
	mux.HandleFunc("/api/task", s.getTask)
	mux.HandleFunc("/api/cancel", s.cancel)
	return mux
}

// apiReq is the common JSON body for API requests.
type apiReq struct {
	Server    string `json:"server"`
	Transport string `json:"transport"`
	Message   string `json:"message"`
	SessionID string `json:"session_id"`
	Legacy    bool   `json:"legacy"`
	ID        string `json:"id"`
}

func (s *Server) buildClient(server, transport string) (*client.A2AClient, string, error) {
	url, err := config.ResolveServerURL(server)
	if err != nil {
		return nil, "", err
	}
	c, err := client.BuildClient(url, transport)
	return c, url, err
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// GET /api/agent-card?server=...&transport=...
func (s *Server) agentCard(w http.ResponseWriter, r *http.Request) {
	server := r.URL.Query().Get("server")
	transport := r.URL.Query().Get("transport")
	if server == "" {
		server = "http://localhost:8000"
	}
	if transport == "" {
		transport = "http"
	}
	c, url, err := s.buildClient(server, transport)
	if err != nil {
		writeErr(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer c.Close()
	card := c.FetchAgentCard(url)
	writeJSON(w, map[string]interface{}{"card": card})
}

// POST /api/send
func (s *Server) send(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req apiReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, err.Error(), http.StatusBadRequest)
		return
	}
	c, _, err := s.buildClient(req.Server, req.Transport)
	if err != nil {
		writeErr(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer c.Close()

	msg := client.MakeTextMessage(req.Message, uuid.New().String())

	if !req.Legacy {
		if resp, err := c.SendMessage(msg); err == nil {
			if resp.Task != nil {
				writeJSON(w, map[string]interface{}{"task": resp.Task})
			} else {
				writeJSON(w, map[string]interface{}{"message": resp.Message})
			}
			return
		}
	}

	sid := req.SessionID
	task, err := c.SendTask(model.TaskSendParams{
		ID:        uuid.New().String(),
		SessionID: &sid,
		Message:   msg,
	})
	if err != nil {
		writeErr(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]interface{}{"task": task})
}

// POST /api/stream  — responds with text/event-stream
func (s *Server) stream(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req apiReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, err.Error(), http.StatusBadRequest)
		return
	}
	c, _, err := s.buildClient(req.Server, req.Transport)
	if err != nil {
		writeErr(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer c.Close()

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeErr(w, "streaming not supported by this server", http.StatusInternalServerError)
		return
	}

	msg := client.MakeTextMessage(req.Message, uuid.New().String())
	sid := req.SessionID
	params := model.TaskSendParams{
		ID:        uuid.New().String(),
		SessionID: &sid,
		Message:   msg,
	}

	var ch <-chan client.StreamEvent
	ch, err = c.StreamMessage(msg)
	if err != nil {
		ch, err = c.SendSubscribe(params)
	}
	if err != nil {
		writeErr(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")

	emit := func(evType string, data interface{}) {
		b, _ := json.Marshal(map[string]interface{}{"type": evType, "data": data})
		fmt.Fprintf(w, "data: %s\n\n", b)
		flusher.Flush()
	}

	for ev := range ch {
		switch {
		case ev.Status != nil:
			emit("status", ev.Status)
		case ev.Artifact != nil:
			emit("artifact", ev.Artifact)
		case ev.Message != nil:
			emit("message", ev.Message)
		}
	}
}

// GET /api/task?server=...&transport=...&id=...
func (s *Server) getTask(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	c, _, err := s.buildClient(q.Get("server"), q.Get("transport"))
	if err != nil {
		writeErr(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer c.Close()

	task, err := c.GetTask(model.TaskQueryParams{ID: q.Get("id")})
	if err != nil {
		writeErr(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]interface{}{"task": task})
}

// POST /api/cancel
func (s *Server) cancel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req apiReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, err.Error(), http.StatusBadRequest)
		return
	}
	c, _, err := s.buildClient(req.Server, req.Transport)
	if err != nil {
		writeErr(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer c.Close()

	if err := c.CancelTask(model.TaskIDParams{ID: req.ID}); err != nil {
		writeErr(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]interface{}{"ok": true})
}
