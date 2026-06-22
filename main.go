package main

import (
	"bufio"
	"embed"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"strings"

	"github.com/codeyogico/a2a-probe/internal/chat"
	"github.com/codeyogico/a2a-probe/internal/client"
	"github.com/codeyogico/a2a-probe/internal/config"
	"github.com/codeyogico/a2a-probe/internal/model"
	"github.com/codeyogico/a2a-probe/internal/server"
	"github.com/codeyogico/a2a-probe/internal/ui"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

//go:embed web
var webFS embed.FS

const version = "0.2.0"

var (
	flagServer    string
	flagTransport string
	flagDebug     bool
	flagQuiet     bool
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
	return client.BuildClient(url, flagTransport)
}

func sessionID() string { return uuid.New().String() }

// ── send ──────────────────────────────────────────────────────────────────────

func newSendCmd() *cobra.Command {
	var stream bool
	var taskID string

	cmd := &cobra.Command{
		Use:   "send <message>",
		Short: "Send a task to the agent and print the result",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			text := args[0]
			sid := sessionID()
			c, err := buildClient()
			if err != nil {
				return err
			}
			defer c.Close()

			msg := client.MakeTextMessage(text, uuid.New().String())
			params := model.TaskSendParams{
				ID:        taskID,
				SessionID: strPtr(sid),
				Message:   msg,
			}

			if stream {
				ch, err := c.StreamMessage(msg)
				if err != nil {
					ch, err = c.SendSubscribe(params)
					if err != nil {
						ui.PrintError(err.Error())
						return fmt.Errorf("failed")
					}
				}
				for ev := range ch {
					printEvent(ev)
				}
				return nil
			}

			resp, err := c.SendMessage(msg)
			if err == nil {
				if resp.Task != nil {
					ui.PrintTask(resp.Task)
				} else {
					ui.PrintMessage(resp.Message, "")
				}
				return nil
			}
			task, err := c.SendTask(params)
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
	return cmd
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
			c, err := client.BuildClient(url, flagTransport)
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
	case ev.Status != nil:
		ui.PrintStreamStatus(ev.Status)
	case ev.Artifact != nil:
		ui.PrintStreamArtifact(ev.Artifact)
	case ev.Message != nil:
		ui.PrintMessage(ev.Message, "")
	}
}

func strPtr(s string) *string { return &s }
