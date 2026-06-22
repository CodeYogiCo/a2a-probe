package transport

import (
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

// WebSocketTransport implements JSON-RPC 2.0 over a WebSocket connection.
type WebSocketTransport struct {
	url  string
	conn *websocket.Conn
	ch   chan json.RawMessage
}

// NewWebSocket creates a WebSocketTransport that connects to the given URL.
func NewWebSocket(url string) *WebSocketTransport {
	return &WebSocketTransport{
		url: url,
		ch:  make(chan json.RawMessage, 256),
	}
}

func (t *WebSocketTransport) ensureConnected() error {
	if t.conn != nil {
		return nil
	}
	conn, _, err := websocket.DefaultDialer.Dial(t.url, nil)
	if err != nil {
		return fmt.Errorf("websocket dial %s: %w", t.url, err)
	}
	t.conn = conn

	go func() {
		defer close(t.ch)
		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}
			t.ch <- json.RawMessage(msg)
		}
	}()
	return nil
}

func (t *WebSocketTransport) Call(method string, params json.RawMessage) (json.RawMessage, error) {
	if err := t.ensureConnected(); err != nil {
		return nil, err
	}
	envelope := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  method,
		"params":  params,
		"id":      uuid.New().String(),
	}
	msg, _ := json.Marshal(envelope)
	if err := t.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
		return nil, err
	}
	raw, ok := <-t.ch
	if !ok {
		return nil, fmt.Errorf("websocket closed")
	}
	return parseResponse(raw)
}

func (t *WebSocketTransport) Stream() <-chan json.RawMessage {
	return t.ch
}

func (t *WebSocketTransport) Close() error {
	if t.conn != nil {
		err := t.conn.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
		t.conn.Close()
		t.conn = nil
		return err
	}
	return nil
}
