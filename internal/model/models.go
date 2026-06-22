package model

import "encoding/json"

// TaskState enumerates task lifecycle states.
type TaskState string

const (
	TaskStateSubmitted TaskState = "submitted"
	TaskStateWorking   TaskState = "working"
	TaskStateCompleted TaskState = "completed"
	TaskStateFailed    TaskState = "failed"
	TaskStateCanceled  TaskState = "canceled"
	TaskStateUnknown   TaskState = "unknown"
)

// Role identifies who sent a message.
type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleAgent     Role = "agent"
)

// Message is an A2A message (≥0.4 API).
type Message struct {
	Kind      string            `json:"kind,omitempty"`
	Role      Role              `json:"role"`
	Parts     []json.RawMessage `json:"parts"`
	MessageID string            `json:"messageId,omitempty"`
	Metadata  json.RawMessage   `json:"metadata,omitempty"`
}

// Artifact holds output content from an agent task.
type Artifact struct {
	Name        *string           `json:"name,omitempty"`
	Description *string           `json:"description,omitempty"`
	Parts       []json.RawMessage `json:"parts,omitempty"`
	Index       int               `json:"index"`
	Append      *bool             `json:"append,omitempty"`
	LastChunk   *bool             `json:"lastChunk,omitempty"`
	Metadata    json.RawMessage   `json:"metadata,omitempty"`
}

// TaskStatus describes the current state of a task.
type TaskStatus struct {
	State     TaskState `json:"state"`
	Message   *Message  `json:"message,omitempty"`
	Timestamp *string   `json:"timestamp,omitempty"`
}

// Task is an A2A task object.
type Task struct {
	ID        string          `json:"id"`
	SessionID *string         `json:"sessionId,omitempty"`
	Status    TaskStatus      `json:"status"`
	Artifacts []Artifact      `json:"artifacts,omitempty"`
	History   []Message       `json:"history,omitempty"`
	Metadata  json.RawMessage `json:"metadata,omitempty"`
}

// TaskSendParams are the parameters for tasks/send.
type TaskSendParams struct {
	ID                  string                  `json:"id"`
	SessionID           *string                 `json:"sessionId,omitempty"`
	Message             Message                 `json:"message"`
	AcceptedOutputModes []string                `json:"acceptedOutputModes,omitempty"`
	PushNotification    *PushNotificationConfig `json:"pushNotification,omitempty"`
	HistoryLength       *int                    `json:"historyLength,omitempty"`
	Metadata            json.RawMessage         `json:"metadata,omitempty"`
}

// TaskQueryParams are used for tasks/get and tasks/resubscribe.
type TaskQueryParams struct {
	ID            string          `json:"id"`
	HistoryLength *int            `json:"historyLength,omitempty"`
	Metadata      json.RawMessage `json:"metadata,omitempty"`
}

// TaskIDParams are used for tasks/cancel.
type TaskIDParams struct {
	ID       string          `json:"id"`
	Metadata json.RawMessage `json:"metadata,omitempty"`
}

// PushNotificationConfig configures push notifications for a task.
type PushNotificationConfig struct {
	ID             string          `json:"id"`
	URL            string          `json:"url"`
	Token          *string         `json:"token,omitempty"`
	Authentication json.RawMessage `json:"authentication,omitempty"`
}

// TaskStatusUpdateEvent is emitted during streaming when task state changes.
type TaskStatusUpdateEvent struct {
	ID       string          `json:"id"`
	Status   TaskStatus      `json:"status"`
	Final    bool            `json:"final"`
	Metadata json.RawMessage `json:"metadata,omitempty"`
}

// TaskArtifactUpdateEvent is emitted during streaming when an artifact is ready.
type TaskArtifactUpdateEvent struct {
	ID       string          `json:"id"`
	Artifact Artifact        `json:"artifact"`
	Metadata json.RawMessage `json:"metadata,omitempty"`
}

// JsonRpcRequest is a JSON-RPC 2.0 request envelope.
type JsonRpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
	ID      string          `json:"id"`
}

// JsonRpcResponse is a JSON-RPC 2.0 response envelope.
type JsonRpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *JsonRpcError   `json:"error,omitempty"`
	ID      json.RawMessage `json:"id,omitempty"`
}

// JsonRpcError is the error object in a JSON-RPC 2.0 response.
type JsonRpcError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// AgentCard is the well-known agent descriptor at /.well-known/agent.json.
type AgentCard struct {
	Name         *string           `json:"name,omitempty"`
	Description  *string           `json:"description,omitempty"`
	URL          *string           `json:"url,omitempty"`
	Version      *string           `json:"version,omitempty"`
	Capabilities *AgentCapabilities `json:"capabilities,omitempty"`
	Skills       []AgentSkill      `json:"skills,omitempty"`
}

// AgentCapabilities describes what an agent supports.
type AgentCapabilities struct {
	Streaming              *bool `json:"streaming,omitempty"`
	PushNotifications      *bool `json:"pushNotifications,omitempty"`
	StateTransitionHistory *bool `json:"stateTransitionHistory,omitempty"`
}

// AgentSkill describes a skill exposed by an agent.
type AgentSkill struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description *string  `json:"description,omitempty"`
	Tags        []string `json:"tags,omitempty"`
}

// ServerConfig is a named server entry in the CLI config.
type ServerConfig struct {
	URL       string `json:"url"`
	Transport string `json:"transport,omitempty"`
}

// CliConfig is stored at ~/.a2a/config.json.
type CliConfig struct {
	Servers       map[string]ServerConfig `json:"servers,omitempty"`
	DefaultServer *string                 `json:"default_server,omitempty"`
}
