package ui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/codeyogico/a2a-probe/internal/debug"
	"github.com/codeyogico/a2a-probe/internal/model"
)

// ANSI color helpers
const (
	reset  = "\033[0m"
	bold   = "\033[1m"
	dim    = "\033[2m"
	red    = "\033[31m"
	green  = "\033[32m"
	yellow = "\033[33m"
	blue   = "\033[34m"
	cyan   = "\033[36m"
	white  = "\033[37m"
)

func color(c, s string) string { return c + s + reset }
func Bold(s string) string     { return color(bold, s) }
func Dim(s string) string      { return color(dim, s) }
func Red(s string) string      { return color(red, s) }
func Green(s string) string    { return color(green, s) }
func Yellow(s string) string   { return color(yellow, s) }
func Blue(s string) string     { return color(blue, s) }
func Cyan(s string) string     { return color(cyan, s) }
func White(s string) string    { return color(white, s) }

func stateColor(state model.TaskState) func(string) string {
	switch state {
	case model.TaskStateCompleted:
		return Green
	case model.TaskStateFailed:
		return Red
	case model.TaskStateCanceled:
		return Yellow
	case model.TaskStateWorking:
		return Cyan
	default:
		return White
	}
}

// PrintWelcomeBanner prints the interactive chat banner.
func PrintWelcomeBanner(sessionID string) {
	fmt.Println()
	fmt.Println(Bold("╔══════════════════════════════════════╗"))
	fmt.Println(Bold("║       A2A Probe — Go Edition          ║"))
	fmt.Println(Bold("║    Agent-to-Agent Protocol v0.3.0     ║"))
	fmt.Println(Bold("╚══════════════════════════════════════╝"))
	fmt.Println(Dim("  Session: " + sessionID))
	fmt.Println(Dim("  Type 'help' or '/help' for commands"))
	fmt.Println()
}

// PrintTask prints a task summary.
func PrintTask(task *model.Task) {
	col := stateColor(task.Status.State)
	fmt.Printf("%s %s\n", Bold("Task"), White(task.ID))
	fmt.Printf("  Status: %s\n", col(string(task.Status.State)))
	if task.Status.Message != nil {
		PrintMessage(task.Status.Message, "  ")
	}
	for i, artifact := range task.Artifacts {
		name := "unnamed"
		if artifact.Name != nil {
			name = *artifact.Name
		}
		fmt.Printf("%s\n", Dim(fmt.Sprintf("  ─── artifact[%d]: %s ───", i, name)))
		PrintArtifactParts(artifact.Parts)
	}
}

// PrintMessage prints a message with a role prefix.
func PrintMessage(msg *model.Message, indent string) {
	var prefix string
	switch msg.Role {
	case model.RoleUser:
		prefix = Bold(Blue("You:      "))
	default:
		prefix = Bold(Green("Agent:    "))
	}
	text := ExtractText(msg.Parts)
	fmt.Printf("%s%s%s\n", indent, prefix, text)
}

// PrintStreamStatus prints a streaming status update.
func PrintStreamStatus(event *model.TaskStatusUpdateEvent) {
	col := stateColor(event.Status.State)
	fmt.Printf("\r%s%s%s ", Dim("["), col(string(event.Status.State)), Dim("]"))
	if event.Status.Message != nil {
		fmt.Print(ExtractText(event.Status.Message.Parts))
	}
	if event.Final {
		fmt.Println()
	}
}

// PrintStreamArtifact prints a streaming artifact update.
func PrintStreamArtifact(event *model.TaskArtifactUpdateEvent) {
	if event.Artifact.Index == 0 && (event.Artifact.Append == nil || !*event.Artifact.Append) {
		fmt.Println()
	}
	PrintArtifactParts(event.Artifact.Parts)
}

// PrintArtifactParts prints the parts of an artifact.
func PrintArtifactParts(parts []json.RawMessage) {
	for _, p := range parts {
		var obj map[string]json.RawMessage
		if json.Unmarshal(p, &obj) != nil {
			fmt.Println(string(p))
			continue
		}
		kind := rawString(obj, "kind")
		if kind == "" {
			kind = rawString(obj, "type")
		}
		switch kind {
		case "text":
			fmt.Print(rawString(obj, "text"))
		case "file":
			fileRaw, _ := obj["file"]
			var fileObj map[string]json.RawMessage
			if json.Unmarshal(fileRaw, &fileObj) == nil {
				uri := rawString(fileObj, "uri")
				name := rawString(fileObj, "name")
				label := name
				if label == "" {
					label = uri
				}
				if label == "" {
					label = "unknown"
				}
				fmt.Println(Cyan("[file: " + label + "]"))
			}
		case "data":
			// Structured JSON payload — pretty-print the inner data object on
			// its own line (a preceding text part may have left the cursor mid-line).
			if d, ok := obj["data"]; ok {
				fmt.Printf("\n%s\n", Dim(prettyJSON(d)))
			} else {
				fmt.Printf("\n%s\n", Dim(prettyJSON(p)))
			}
		default:
			fmt.Printf("\n%s\n", Dim(prettyJSON(p)))
		}
	}
}

// prettyJSON indents a raw JSON value for human reading, falling back to the
// original bytes if it cannot be parsed.
func prettyJSON(raw json.RawMessage) string {
	var buf bytes.Buffer
	if err := json.Indent(&buf, raw, "  ", "  "); err != nil {
		return string(raw)
	}
	return buf.String()
}

// ExtractText joins the text of all parts in a message.
func ExtractText(parts []json.RawMessage) string {
	var sb strings.Builder
	for _, p := range parts {
		var obj map[string]json.RawMessage
		if json.Unmarshal(p, &obj) != nil {
			sb.WriteString(string(p))
			continue
		}
		kind := rawString(obj, "kind")
		if kind == "" {
			kind = rawString(obj, "type")
		}
		if kind == "text" {
			sb.WriteString(rawString(obj, "text"))
		} else {
			sb.WriteString(string(p))
		}
	}
	return sb.String()
}

func rawString(obj map[string]json.RawMessage, key string) string {
	v, ok := obj[key]
	if !ok {
		return ""
	}
	var s string
	json.Unmarshal(v, &s)
	return s
}

// PrintError prints an error message in red.
func PrintError(msg string) { fmt.Println(Red("Error: " + msg)) }

// PrintInfo prints an informational message in dim style.
func PrintInfo(msg string) { fmt.Println(Dim(msg)) }

// PrintSuccess prints a success message in green.
func PrintSuccess(msg string) { fmt.Println(Green(msg)) }

// Spinner is an animated "working" indicator drawn on stderr while a request is
// in flight. On a non-TTY (pipes, CI) it is a no-op so output stays clean.
type Spinner struct {
	stop chan struct{}
	done chan struct{}
}

// StartSpinner begins animating until Stop is called.
func StartSpinner(label string) *Spinner {
	s := &Spinner{stop: make(chan struct{}), done: make(chan struct{})}
	// Skip the animation on a non-TTY, or when debug logging is on (its lines
	// would clobber the spinner).
	if !stdoutIsTTY() || debug.Enabled() {
		close(s.done)
		return s
	}
	go func() {
		defer close(s.done)
		frames := []rune("⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏")
		start := time.Now()
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for i := 0; ; i++ {
			select {
			case <-s.stop:
				fmt.Fprint(os.Stderr, "\r\033[K") // clear the line
				return
			case <-ticker.C:
				elapsed := time.Since(start).Round(time.Second)
				fmt.Fprintf(os.Stderr, "\r%s %s %s",
					Cyan(string(frames[i%len(frames)])), label, Dim("("+elapsed.String()+")"))
			}
		}
	}()
	return s
}

// Stop halts the spinner and clears its line. Safe to call exactly once.
func (s *Spinner) Stop() {
	select {
	case <-s.done: // non-TTY no-op, already finished
		return
	default:
	}
	close(s.stop)
	<-s.done
}

func stdoutIsTTY() bool {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}
