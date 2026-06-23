package chat

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/codeyogico/a2a-probe/internal/client"
	"github.com/codeyogico/a2a-probe/internal/model"
	"github.com/codeyogico/a2a-probe/internal/ui"
	"github.com/google/uuid"
)

// Handler drives an interactive chat session.
type Handler struct {
	c         *client.A2AClient
	sessionID string
	serverURL string
}

// New creates a new ChatHandler.
func New(c *client.A2AClient, sessionID, serverURL string) *Handler {
	return &Handler{c: c, sessionID: sessionID, serverURL: serverURL}
}

// Run starts the interactive REPL loop.
func (h *Handler) Run() error {
	ui.PrintWelcomeBanner(h.sessionID)
	ui.PrintInfo("Connected to: " + h.serverURL)

	card := h.c.FetchAgentCard(h.serverURL)
	if card != nil {
		name := "unknown"
		if card.Name != nil {
			name = *card.Name
		}
		ver := ""
		if card.Version != nil {
			ver = " v" + *card.Version
		}
		ui.PrintInfo("Agent: " + name + ver)
		if card.Description != nil {
			ui.PrintInfo(*card.Description)
		}
	}
	ui.PrintInfo("")

	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("> ")
		if !scanner.Scan() {
			break
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		switch {
		case line == "exit" || line == "quit" || line == "/exit" || line == "/quit":
			return nil
		case line == "help" || line == "/help":
			printHelp()
		case strings.HasPrefix(line, "/get "):
			h.handleGet(strings.TrimSpace(strings.TrimPrefix(line, "/get ")))
		case strings.HasPrefix(line, "/cancel "):
			h.handleCancel(strings.TrimSpace(strings.TrimPrefix(line, "/cancel ")))
		case strings.HasPrefix(line, "/watch "):
			h.handleWatch(strings.TrimSpace(strings.TrimPrefix(line, "/watch ")))
		default:
			h.handleSend(line, card)
		}
	}
	return scanner.Err()
}

func (h *Handler) handleSend(text string, card *model.AgentCard) {
	msg := client.MakeTextMessage(text, uuid.New().String())
	params := model.TaskSendParams{
		ID:        uuid.New().String(),
		SessionID: strPtr(h.sessionID),
		Message:   msg,
	}

	canStream := card != nil && card.Capabilities != nil &&
		card.Capabilities.Streaming != nil && *card.Capabilities.Streaming

	sp := ui.StartSpinner()

	if canStream {
		ch, err := h.c.StreamMessage(msg)
		if err != nil {
			ch, err = h.c.SendSubscribe(params)
		}
		sp.Stop()
		if err != nil {
			ui.PrintError(err.Error())
			return
		}
		for ev := range ch {
			handleEvent(ev)
		}
		return
	}

	resp, err := h.c.SendMessage(msg)
	if err != nil {
		task, err2 := h.c.SendTask(params)
		sp.Stop()
		if err2 != nil {
			ui.PrintError(err2.Error())
			return
		}
		ui.PrintTask(task)
		return
	}
	sp.Stop()
	if resp.Task != nil {
		ui.PrintTask(resp.Task)
	} else {
		ui.PrintMessage(resp.Message, "")
	}
}

func (h *Handler) handleGet(taskID string) {
	task, err := h.c.GetTask(model.TaskQueryParams{ID: taskID})
	if err != nil {
		ui.PrintError(err.Error())
		return
	}
	ui.PrintTask(task)
}

func (h *Handler) handleCancel(taskID string) {
	if err := h.c.CancelTask(model.TaskIDParams{ID: taskID}); err != nil {
		ui.PrintError(err.Error())
		return
	}
	ui.PrintSuccess("Task " + taskID + " cancelled.")
}

func (h *Handler) handleWatch(taskID string) {
	ch, err := h.c.Resubscribe(model.TaskQueryParams{ID: taskID})
	if err != nil {
		ui.PrintError(err.Error())
		return
	}
	for ev := range ch {
		handleEvent(ev)
	}
}

func handleEvent(ev client.StreamEvent) {
	switch {
	case ev.Task != nil:
		ui.PrintTask(ev.Task)
	case ev.Status != nil:
		ui.PrintStreamStatus(ev.Status)
	case ev.Artifact != nil:
		ui.PrintStreamArtifact(ev.Artifact)
	case ev.Message != nil:
		ui.PrintMessage(ev.Message, "")
	}
}

func printHelp() {
	fmt.Println(`Commands:
  <message>          Send a message to the agent
  /get <id>          Retrieve task status by ID
  /cancel <id>       Cancel a running task
  /watch <id>        Stream updates for a task
  /help              Show this help
  exit               Quit`)
}

func strPtr(s string) *string { return &s }
