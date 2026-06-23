// Package debug provides opt-in, timestamped tracing. It is enabled by the
// --debug flag (or at runtime via the web UI) and used across the transport,
// client, and server layers to log exactly what is happening on the wire.
//
// Log lines are written to stderr and, additionally, broadcast to any
// subscribers (used by the web UI's live debug panel).
package debug

import (
	"fmt"
	"os"
	"sync"
	"time"
)

var (
	mu          sync.Mutex
	enabled     bool
	subscribers = map[int]chan string{}
	nextSub     int
)

// Enable turns debug logging on.
func Enable() { SetEnabled(true) }

// SetEnabled turns debug logging on or off at runtime.
func SetEnabled(v bool) {
	mu.Lock()
	enabled = v
	mu.Unlock()
}

// Enabled reports whether debug logging is on.
func Enabled() bool {
	mu.Lock()
	defer mu.Unlock()
	return enabled
}

// Subscribe registers a listener for log lines. It returns the channel and a
// cancel func that must be called to unsubscribe.
func Subscribe() (<-chan string, func()) {
	ch := make(chan string, 256)
	mu.Lock()
	id := nextSub
	nextSub++
	subscribers[id] = ch
	mu.Unlock()

	cancel := func() {
		mu.Lock()
		if c, ok := subscribers[id]; ok {
			delete(subscribers, id)
			close(c)
		}
		mu.Unlock()
	}
	return ch, cancel
}

// Logf writes a timestamped debug line to stderr and all subscribers when
// enabled.
func Logf(format string, args ...any) {
	mu.Lock()
	on := enabled
	subs := make([]chan string, 0, len(subscribers))
	for _, ch := range subscribers {
		subs = append(subs, ch)
	}
	mu.Unlock()

	if !on {
		return
	}

	ts := time.Now().Format("15:04:05.000")
	msg := fmt.Sprintf(format, args...)
	line := fmt.Sprintf("[%s] %s", ts, msg)

	fmt.Fprintf(os.Stderr, "\033[2m[debug %s] %s\033[0m\n", ts, msg)

	// Non-blocking fan-out: never let a slow subscriber stall the request path.
	for _, ch := range subs {
		select {
		case ch <- line:
		default:
		}
	}
}

// Truncate shortens long payloads for readable logs.
func Truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + fmt.Sprintf("… (%d bytes total)", len(s))
}
