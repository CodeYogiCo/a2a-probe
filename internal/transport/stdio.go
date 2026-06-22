package transport

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"

	"github.com/google/uuid"
)

// StdioTransport implements JSON-RPC 2.0 over stdin/stdout.
type StdioTransport struct {
	reader *bufio.Reader
}

// NewStdio creates a StdioTransport reading from stdin and writing to stdout.
func NewStdio() *StdioTransport {
	return &StdioTransport{
		reader: bufio.NewReader(os.Stdin),
	}
}

func (t *StdioTransport) Call(method string, params json.RawMessage) (json.RawMessage, error) {
	envelope := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  method,
		"params":  params,
		"id":      uuid.New().String(),
	}
	line, _ := json.Marshal(envelope)
	fmt.Fprintf(os.Stdout, "%s\n", line)

	respLine, err := t.reader.ReadString('\n')
	if err != nil {
		return nil, fmt.Errorf("EOF on stdin — server closed the pipe")
	}
	return parseResponse([]byte(respLine))
}

func (t *StdioTransport) Stream() <-chan json.RawMessage {
	ch := make(chan json.RawMessage, 256)
	go func() {
		defer close(ch)
		for {
			line, err := t.reader.ReadString('\n')
			if err != nil {
				break
			}
			trimmed := []byte(line)
			if len(trimmed) > 0 {
				ch <- json.RawMessage(trimmed)
			}
		}
	}()
	return ch
}

func (t *StdioTransport) Close() error { return nil }
