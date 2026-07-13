package main

import (
	"bufio"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/codeyogico/a2a-probe/internal/chat"
	"github.com/codeyogico/a2a-probe/internal/client"
	"github.com/codeyogico/a2a-probe/internal/config"
	"github.com/codeyogico/a2a-probe/internal/debug"
	"github.com/codeyogico/a2a-probe/internal/model"
	"github.com/codeyogico/a2a-probe/internal/server"
	"github.com/codeyogico/a2a-probe/internal/ui"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

//go:embed web
var webFS embed.FS

const version = "0.3.1"

var (
	flagServer    string
	flagTransport string
	flagDebug     bool
	flagQuiet     bool
	flagHeaders   []string
	flagBearer    string
	flagAPIKey    string
)

func main() {
	root := &cobra.Command{
		Use:     "a2a-probe",
		Short:   "Command-line client for the A2A (Agent-to-Agent) Protocol v0.3.0",
		Version: version,
	}
	root.PersistentFlags().StringVarP(&flagServer, "server", "s", "http://localhost:8000",
		"Server URL or config alias")
	root.PersistentFlags().StringVarP(&flagTransport, "transport", "t", "http",
		"Transport: http | sse | ws | stdio")
	root.PersistentFlags().BoolVar(&flagDebug, "debug", false, "Enable debug logging")
	root.PersistentFlags().BoolVarP(&flagQuiet, "quiet", "q", false, "Suppress non-essential output")
	root.PersistentFlags().StringArrayVarP(&flagHeaders, "header", "H", nil,
		"Extra request header 'Key: Value' (repeatable)")
	root.PersistentFlags().StringVar(&flagBearer, "bearer", "", "Bearer token (sets Authorization: Bearer …)")
	root.PersistentFlags().StringVar(&flagAPIKey, "api-key", "", "API key (sets X-API-Key header)")

	root.PersistentPreRun = func(cmd *cobra.Command, args []string) {
		if flagDebug {
			debug.Enable()
			debug.Logf("debug logging enabled (transport=%s)", flagTransport)
		}
	}

	root.AddCommand(
		newSendCmd(),
		newGetCmd(),
		newCancelCmd(),
		newWatchCmd(),
		newChatCmd(),
		newStdioCmd(),
		newServeCmd(),
		newConfigCmd(),
	)

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

// buildClient resolves the server URL and creates an A2AClient.
func buildClient() (*client.A2AClient, error) {
	url, err := config.ResolveServerURL(flagServer)
	if err != nil {
		return nil, err
	}
	headers, err := authHeaders()
	if err != nil {
		return nil, err
	}
	return client.BuildClientWithHeaders(url, flagTransport, headers)
}

// authHeaders builds the request headers from --header/--bearer/--api-key.
func authHeaders() (map[string]string, error) {
	h := map[string]string{}
	for _, raw := range flagHeaders {
		k, v, ok := strings.Cut(raw, ":")
		if !ok {
			return nil, fmt.Errorf("invalid --header %q (want 'Key: Value')", raw)
		}
		h[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}
	if flagBearer != "" {
		h["Authorization"] = "Bearer " + flagBearer
	}
	if flagAPIKey != "" {
		h["X-API-Key"] = flagAPIKey
	}
	if len(h) == 0 {
		return nil, nil
	}
	return h, nil
}

func sessionID() string { return uuid.New().String() }

// ── send ──────────────────────────────────────────────────────────────────────

func newSendCmd() *cobra.Command {
	var stream bool
	var taskID string
	var dataFlag, fileFlag, metaFlag string

	cmd := &cobra.Command{
		Use:   "send [message]",
		Short: "Send a message (text and/or structured data/file) to the agent",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			sid := sessionID()

			parts, err := buildParts(args, dataFlag, fileFlag)
			if err != nil {
				return err
			}
			if len(parts) == 0 {
				return fmt.Errorf("nothing to send: provide a message, --data, or --file")
			}
			var meta json.RawMessage
			if metaFlag != "" {
				if meta, err = readJSONArg(metaFlag); err != nil {
					return fmt.Errorf("--metadata: %w", err)
				}
			}

			c, err := buildClient()
			if err != nil {
				return err
			}
			defer c.Close()

			msg := client.MakeMessage(uuid.New().String(), parts, meta)
			params := model.TaskSendParams{
				ID:        taskID,
				SessionID: strPtr(sid),
				Message:   msg,
			}

			if stream {
				sp := ui.StartSpinner()
				ch, err := c.StreamMessage(msg)
				if err != nil {
					ch, err = c.SendSubscribe(params)
					if err != nil {
						sp.Stop()
						ui.PrintError(err.Error())
						return fmt.Errorf("failed")
					}
				}
				sp.Stop()
				for ev := range ch {
					printEvent(ev)
				}
				return nil
			}

			sp := ui.StartSpinner()
			resp, err := c.SendMessage(msg)
			if err == nil {
				sp.Stop()
				if resp.Task != nil {
					ui.PrintTask(resp.Task)
				} else {
					ui.PrintMessage(resp.Message, "")
				}
				return nil
			}
			task, err := c.SendTask(params)
			sp.Stop()
			if err != nil {
				ui.PrintError(err.Error())
				return fmt.Errorf("failed")
			}
			ui.PrintTask(task)
			return nil
		},
	}
	cmd.Flags().BoolVar(&stream, "stream", false, "Use streaming (sendSubscribe)")
	cmd.Flags().StringVar(&taskID, "id", uuid.New().String(), "Task ID")
	cmd.Flags().StringVar(&dataFlag, "data", "", "Attach a structured JSON data part (inline JSON or @file.json)")
	cmd.Flags().StringVar(&fileFlag, "file", "", "Attach a file part (local path or http(s) URI)")
	cmd.Flags().StringVar(&metaFlag, "metadata", "", "Message metadata as JSON (inline or @file.json)")
	return cmd
}

// buildParts assembles message parts from the text arg and --data/--file flags.
func buildParts(args []string, dataFlag, fileFlag string) ([]json.RawMessage, error) {
	var parts []json.RawMessage
	if len(args) == 1 && args[0] != "" {
		parts = append(parts, client.TextPart(args[0]))
	}
	if dataFlag != "" {
		raw, err := readJSONArg(dataFlag)
		if err != nil {
			return nil, fmt.Errorf("--data: %w", err)
		}
		parts = append(parts, client.DataPart(raw))
	}
	if fileFlag != "" {
		p, err := filePart(fileFlag)
		if err != nil {
			return nil, fmt.Errorf("--file: %w", err)
		}
		parts = append(parts, p)
	}
	return parts, nil
}

// readJSONArg reads a JSON value given inline or as @path, validating it.
func readJSONArg(v string) (json.RawMessage, error) {
	s := v
	if strings.HasPrefix(v, "@") {
		b, err := os.ReadFile(v[1:])
		if err != nil {
			return nil, err
		}
		s = string(b)
	}
	if !json.Valid([]byte(s)) {
		return nil, fmt.Errorf("invalid JSON")
	}
	return json.RawMessage(s), nil
}

// filePart builds a file part from an http(s) URI or a local file path.
func filePart(v string) (json.RawMessage, error) {
	if strings.HasPrefix(v, "http://") || strings.HasPrefix(v, "https://") {
		return client.FilePartURI(v, path.Base(v), ""), nil
	}
	b, err := os.ReadFile(v)
	if err != nil {
		return nil, err
	}
	return client.FilePartBytes(b, filepath.Base(v), mime.TypeByExtension(filepath.Ext(v))), nil
}

// ── get ───────────────────────────────────────────────────────────────────────

func newGetCmd() *cobra.Command {
	var historyLen int
	var hasHistory bool

	cmd := &cobra.Command{
		Use:   "get <task-id>",
		Short: "Retrieve a task by ID",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			defer c.Close()

			p := model.TaskQueryParams{ID: args[0]}
			if hasHistory {
				p.HistoryLength = &historyLen
			}
			task, err := c.GetTask(p)
			if err != nil {
				ui.PrintError(err.Error())
				return fmt.Errorf("failed")
			}
			ui.PrintTask(task)
			return nil
		},
	}
	cmd.Flags().IntVar(&historyLen, "history", 0, "Number of history entries to include")
	cmd.Flags().BoolVar(&hasHistory, "with-history", false, "Include history")
	return cmd
}

// ── cancel ────────────────────────────────────────────────────────────────────

func newCancelCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "cancel <task-id>",
		Short: "Cancel a running task",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			defer c.Close()

			if err := c.CancelTask(model.TaskIDParams{ID: args[0]}); err != nil {
				ui.PrintError(err.Error())
				return fmt.Errorf("failed")
			}
			ui.PrintSuccess("Task " + args[0] + " cancelled.")
			return nil
		},
	}
}

// ── watch ─────────────────────────────────────────────────────────────────────

func newWatchCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "watch <task-id>",
		Short: "Stream live updates for an existing task",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := buildClient()
			if err != nil {
				return err
			}
			defer c.Close()

			ch, err := c.Resubscribe(model.TaskQueryParams{ID: args[0]})
			if err != nil {
				ui.PrintError(err.Error())
				return fmt.Errorf("failed")
			}
			for ev := range ch {
				printEvent(ev)
			}
			return nil
		},
	}
}

// ── chat ──────────────────────────────────────────────────────────────────────

func newChatCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "chat",
		Short: "Enter interactive chat mode",
		RunE: func(cmd *cobra.Command, args []string) error {
			url, err := config.ResolveServerURL(flagServer)
			if err != nil {
				return err
			}
			c, err := buildClient()
			if err != nil {
				return err
			}
			defer c.Close()
			return chat.New(c, sessionID(), url).Run()
		},
	}
}

// ── stdio ─────────────────────────────────────────────────────────────────────

func newStdioCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stdio",
		Short: "JSON-RPC 2.0 over stdin/stdout — for CI/CD pipelines",
		RunE: func(cmd *cobra.Command, args []string) error {
			scanner := bufio.NewScanner(os.Stdin)
			for scanner.Scan() {
				line := strings.TrimSpace(scanner.Text())
				if line == "" {
					continue
				}
				fmt.Println(line)
			}
			return scanner.Err()
		},
	}
}

// ── serve ─────────────────────────────────────────────────────────────────────

func newServeCmd() *cobra.Command {
	var port int
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start a local web UI for interacting with A2A agents",
		RunE: func(cmd *cobra.Command, args []string) error {
			sub, err := fs.Sub(webFS, "web")
			if err != nil {
				return err
			}
			srv := server.New(sub)
			addr := fmt.Sprintf(":%d", port)
			ui.PrintSuccess(fmt.Sprintf("Web UI → http://localhost%s", addr))
			return http.ListenAndServe(addr, srv.Handler())
		},
	}
	cmd.Flags().IntVarP(&port, "port", "p", 7070, "Port to listen on")
	return cmd
}

// ── config ────────────────────────────────────────────────────────────────────

func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage server configuration (~/.a2a/config.json)",
	}
	cmd.AddCommand(newConfigAddCmd(), newConfigRemoveCmd(), newConfigListCmd())
	return cmd
}

func newConfigAddCmd() *cobra.Command {
	var transport string
	cmd := &cobra.Command{
		Use:   "add <name> <url>",
		Short: "Add or update a named server",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			name, url := args[0], args[1]
			cfg := config.Load()
			if cfg.Servers == nil {
				cfg.Servers = map[string]model.ServerConfig{}
			}
			cfg.Servers[name] = model.ServerConfig{URL: url, Transport: transport}
			if err := config.Save(cfg); err != nil {
				return err
			}
			ui.PrintSuccess(fmt.Sprintf("Server '%s' saved → %s", name, url))
			return nil
		},
	}
	cmd.Flags().StringVarP(&transport, "transport", "t", "http", "Transport: http | sse | ws")
	return cmd
}

func newConfigRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove <name>",
		Short: "Remove a named server",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := config.Load()
			delete(cfg.Servers, args[0])
			if err := config.Save(cfg); err != nil {
				return err
			}
			ui.PrintSuccess("Server '" + args[0] + "' removed.")
			return nil
		},
	}
}

func newConfigListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List configured servers",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := config.Load()
			if len(cfg.Servers) == 0 {
				ui.PrintInfo("No servers configured. Use: a2a-probe config add <name> <url>")
				return nil
			}
			fmt.Println(ui.Bold("Configured servers:"))
			for name, srv := range cfg.Servers {
				fmt.Printf("  %s  %s  [%s]\n", name, srv.URL, srv.Transport)
			}
			return nil
		},
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

func printEvent(ev client.StreamEvent) {
	switch {
	case ev.Task != nil:
		ui.PrintTask(ev.Task)
	case ev.Status != nil:
		ui.PrintStreamStatus(ev.Status)
	case ev.Artifact != nil:
		ui.PrintStreamArtifact(ev.Artifact)
	case ev.Message != nil:
		ui.PrintMessage(ev.Message, "")
	case ev.Raw != nil:
		debug.Logf("unhandled stream event: %s", debug.Truncate(string(ev.Raw), 2000))
	}
}

func strPtr(s string) *string { return &s }
