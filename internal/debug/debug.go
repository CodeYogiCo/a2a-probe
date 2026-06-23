// Package debug provides opt-in, timestamped tracing to stderr. It is enabled
// by the --debug flag and used across the transport, client, and server layers
// to log exactly what is happening on the wire.
package debug

import (
	"fmt"
	"os"
	"sync"
	"time"
)

var (
	mu      sync.Mutex
	enabled bool
)

// Enable turns debug logging on.
func Enable() {
	mu.Lock()
	enabled = true
	mu.Unlock()
}

// Enabled reports whether debug logging is on.
func Enabled() bool {
	mu.Lock()
	defer mu.Unlock()
	return enabled
}

// Logf writes a timestamped, dimmed debug line to stderr when enabled.
func Logf(format string, args ...any) {
	if !Enabled() {
		return
	}
	ts := time.Now().Format("15:04:05.000")
	msg := fmt.Sprintf(format, args...)
	// Serialize writes so concurrent goroutines don't interleave lines.
	mu.Lock()
	fmt.Fprintf(os.Stderr, "\033[2m[debug %s] %s\033[0m\n", ts, msg)
	mu.Unlock()
}

// Truncate shortens long payloads for readable logs.
func Truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + fmt.Sprintf("… (%d bytes total)", len(s))
}
