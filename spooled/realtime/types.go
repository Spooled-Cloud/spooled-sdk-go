// Package realtime provides SSE and WebSocket clients for real-time Spooled events.
package realtime

import (
	"encoding/json"
	"time"
)

// ConnectionState represents the state of a realtime connection.
type ConnectionState string

const (
	StateDisconnected ConnectionState = "disconnected"
	StateConnecting   ConnectionState = "connecting"
	StateConnected    ConnectionState = "connected"
	StateReconnecting ConnectionState = "reconnecting"
)

// EventType represents the type of realtime event.
type EventType string

const (
	// Canonical event types emitted by the server. The server tags these with
	// PascalCase variant names on the wire (e.g. "JobStatusChange"); the SDK
	// normalizes them to the dotted names below before dispatch. See
	// normalizeEventType and serverEventTypeMap.
	EventJobStatusChanged   EventType = "job.status_changed"
	EventJobCreated         EventType = "job.created"
	EventJobCompleted       EventType = "job.completed"
	EventJobFailed          EventType = "job.failed"
	EventQueueStats         EventType = "queue.stats"
	EventWorkerHeartbeat    EventType = "worker.heartbeat"
	EventWorkerRegistered   EventType = "worker.registered"
	EventWorkerDeregistered EventType = "worker.deregistered"
	EventSystemHealth       EventType = "system.health"
	EventPing               EventType = "ping"
	EventError              EventType = "error"

	// Additional event names retained for backward compatibility. These are not
	// currently emitted by the server but remain valid handler registration keys.
	EventJobStarted     EventType = "job.started"
	EventJobRetrying    EventType = "job.retrying"
	EventJobProgress    EventType = "job.progress"
	EventQueuePaused    EventType = "queue.paused"
	EventQueueResumed   EventType = "queue.resumed"
	EventWorkerJoined   EventType = "worker.joined"
	EventWorkerLeft     EventType = "worker.left"
	EventWorkerActive   EventType = "worker.active"
	EventWorkerInactive EventType = "worker.inactive"
)

// serverEventTypeMap maps the event `type` values the server puts on the wire to
// the SDK's canonical dotted EventType names. The server emits PascalCase
// variant names (e.g. "JobCreated"); the SSE transport additionally labels the
// stream with dotted event names from the server's own event_type() (which uses
// "job.status" for a status change). Both forms are mapped to the same canonical
// EventType so typed handlers registered against the dotted names fire
// regardless of which transport or spelling the server used.
var serverEventTypeMap = map[EventType]EventType{
	// PascalCase variants (WebSocket + SSE data body `type` field).
	"JobStatusChange":    EventJobStatusChanged,
	"JobCreated":         EventJobCreated,
	"JobCompleted":       EventJobCompleted,
	"JobFailed":          EventJobFailed,
	"QueueStats":         EventQueueStats,
	"WorkerHeartbeat":    EventWorkerHeartbeat,
	"WorkerRegistered":   EventWorkerRegistered,
	"WorkerDeregistered": EventWorkerDeregistered,
	"SystemHealth":       EventSystemHealth,
	"Ping":               EventPing,
	"Error":              EventError,
	// SSE dotted event-line name that differs from the canonical name.
	"job.status": EventJobStatusChanged,
}

// normalizeEventType maps a raw server event type to the SDK's canonical dotted
// EventType. Values that are already canonical (or unrecognized) are returned
// unchanged so both current dotted names and future/custom event kinds still
// reach catch-all (OnEvent) handlers.
func normalizeEventType(raw EventType) EventType {
	if mapped, ok := serverEventTypeMap[raw]; ok {
		return mapped
	}
	return raw
}

// Event represents a realtime event from the Spooled API.
type Event struct {
	Type      EventType       `json:"type"`
	Timestamp time.Time       `json:"timestamp"`
	Data      json.RawMessage `json:"data"`
}

// JobEvent contains data for job-related events.
type JobEvent struct {
	JobID     string `json:"job_id"`
	QueueName string `json:"queue_name"`
	// Status is the job's status. On job.status_changed the server sends
	// old_status and new_status and no "status", so Status is filled from
	// NewStatus; a handler comparing e.Status to "processing" therefore works.
	Status string `json:"status"`
	// OldStatus and NewStatus are sent with job.status_changed.
	OldStatus  string  `json:"old_status,omitempty"`
	NewStatus  string  `json:"new_status,omitempty"`
	Priority   int     `json:"priority,omitempty"`
	RetryCount int     `json:"retry_count,omitempty"`
	Error      string  `json:"error,omitempty"`
	Progress   float64 `json:"progress,omitempty"`
	// DurationMs is how long the job ran; sent with job.completed.
	DurationMs int64 `json:"duration_ms,omitempty"`
	// Result is the job result when it is a JSON object, and nil otherwise.
	// RawResult has the result whatever its JSON type; a non-object result used to
	// make the whole event fail to decode and silently disappear.
	Result      map[string]any    `json:"-"`
	RawResult   any               `json:"result,omitempty"`
	WorkerID    string            `json:"worker_id,omitempty"`
	Timestamp   time.Time         `json:"timestamp,omitempty"`
	ScheduledAt *time.Time        `json:"scheduled_at,omitempty"`
	StartedAt   *time.Time        `json:"started_at,omitempty"`
	CompletedAt *time.Time        `json:"completed_at,omitempty"`
	FailedAt    *time.Time        `json:"failed_at,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// UnmarshalJSON decodes a job event, mapping new_status onto Status and keeping a
// non-object result instead of failing the event.
func (e *JobEvent) UnmarshalJSON(data []byte) error {
	type jobEventAlias JobEvent
	if err := json.Unmarshal(data, (*jobEventAlias)(e)); err != nil {
		return err
	}
	if e.Status == "" {
		e.Status = e.NewStatus
	}
	e.Result, _ = e.RawResult.(map[string]any)
	return nil
}

// QueueEvent contains data for queue-related events.
type QueueEvent struct {
	QueueName string `json:"queue_name"`
	Reason    string `json:"reason,omitempty"`
}

// WorkerEvent contains data for worker-related events.
type WorkerEvent struct {
	WorkerID   string    `json:"worker_id"`
	QueueName  string    `json:"queue_name"`
	Hostname   string    `json:"hostname,omitempty"`
	Version    string    `json:"version,omitempty"`
	LastSeenAt time.Time `json:"last_seen_at,omitempty"`
}

// SubscriptionFilter specifies which events to receive.
type SubscriptionFilter struct {
	QueueName string   `json:"queue_name,omitempty"`
	JobID     string   `json:"job_id,omitempty"`
	WorkerID  string   `json:"worker_id,omitempty"`
	Events    []string `json:"events,omitempty"`
}

// ConnectionOptions configures a realtime connection.
type ConnectionOptions struct {
	// BaseURL is the base API URL (e.g., "https://api.spooled.cloud")
	BaseURL string
	// WSURL is the WebSocket URL (e.g., "wss://api.spooled.cloud/api/v1/ws")
	WSURL string
	// Token is the JWT or API key for authentication
	Token string
	// APIKey is the API key for authentication (alternative to Token)
	APIKey string
	// AutoReconnect is ignored: reconnecting is on unless DisableAutoReconnect is
	// set. A zero ConnectionOptions used to mean "never reconnect", so a client
	// built from Client.Realtime/RealtimeSSE went silent for good the first time
	// the connection dropped - and the server ends every stream after 30 minutes.
	//
	// Deprecated: use DisableAutoReconnect to turn reconnecting off.
	AutoReconnect bool
	// DisableAutoReconnect turns automatic reconnection off.
	DisableAutoReconnect bool
	// MaxReconnectAttempts is the maximum number of consecutive failed reconnect
	// attempts before the client gives up (default: 10; negative = unlimited). The
	// count restarts once a connection has stayed up for a while.
	MaxReconnectAttempts int
	// MaxMessageBytes is the largest event the client reads (default: 4 MiB). A
	// job.completed event carries the job result, which can be large; an event
	// over the limit drops the connection.
	MaxMessageBytes int
	// ReconnectDelay is the initial delay between reconnect attempts
	ReconnectDelay time.Duration
	// MaxReconnectDelay is the maximum delay between reconnect attempts
	MaxReconnectDelay time.Duration
	// Debug enables debug logging
	Debug bool
	// Logger is a custom logger function
	Logger func(msg string, args ...any)
}

// DefaultConnectionOptions returns options with sensible defaults.
func DefaultConnectionOptions() ConnectionOptions {
	return ConnectionOptions{
		BaseURL:              "https://api.spooled.cloud",
		WSURL:                "wss://api.spooled.cloud/api/v1/ws",
		AutoReconnect:        true,
		MaxReconnectAttempts: 10,
		ReconnectDelay:       1 * time.Second,
		MaxReconnectDelay:    30 * time.Second,
		MaxMessageBytes:      DefaultMaxMessageBytes,
	}
}

// DefaultMaxMessageBytes is the default ConnectionOptions.MaxMessageBytes.
const DefaultMaxMessageBytes = 4 << 20

// stableConnection is how long a connection must stay up before the next drop is
// treated as a fresh failure rather than part of a reconnect loop.
const stableConnection = 10 * time.Second

// applyDefaults fills the zero values of opts that have a default. Reconnecting
// is on unless DisableAutoReconnect says otherwise.
func (o *ConnectionOptions) applyDefaults() {
	defaults := DefaultConnectionOptions()
	if o.BaseURL == "" {
		o.BaseURL = defaults.BaseURL
	}
	if o.ReconnectDelay == 0 {
		o.ReconnectDelay = defaults.ReconnectDelay
	}
	if o.MaxReconnectDelay == 0 {
		o.MaxReconnectDelay = defaults.MaxReconnectDelay
	}
	if o.MaxReconnectAttempts == 0 {
		o.MaxReconnectAttempts = defaults.MaxReconnectAttempts
	}
	if o.MaxMessageBytes <= 0 {
		o.MaxMessageBytes = defaults.MaxMessageBytes
	}
	o.AutoReconnect = !o.DisableAutoReconnect
}

// EventHandler is a callback for handling events.
type EventHandler func(event *Event)

// JobEventHandler is a callback for handling job events.
type JobEventHandler func(event *JobEvent)

// QueueEventHandler is a callback for handling queue events.
type QueueEventHandler func(event *QueueEvent)

// WorkerEventHandler is a callback for handling worker events.
type WorkerEventHandler func(event *WorkerEvent)

// StateChangeHandler is a callback for connection state changes.
type StateChangeHandler func(state ConnectionState)

// RealtimeClient is the interface for realtime connections.
type RealtimeClient interface {
	// Connect establishes the connection
	Connect() error
	// Disconnect closes the connection
	Disconnect() error
	// State returns the current connection state
	State() ConnectionState
	// Subscribe adds a subscription filter (WebSocket only - SSE uses filter at connect time)
	Subscribe(filter SubscriptionFilter) error
	// Unsubscribe removes a subscription (WebSocket only)
	Unsubscribe(filter SubscriptionFilter) error
	// OnEvent registers a handler for all events
	OnEvent(handler EventHandler)
	// OnJobEvent registers a handler for job events
	OnJobEvent(eventType EventType, handler JobEventHandler)
	// OnQueueEvent registers a handler for queue events
	OnQueueEvent(eventType EventType, handler QueueEventHandler)
	// OnWorkerEvent registers a handler for worker events
	OnWorkerEvent(eventType EventType, handler WorkerEventHandler)
	// OnStateChange registers a handler for state changes
	OnStateChange(handler StateChangeHandler)
}

// wsCommand is the client → server command envelope. The server decodes an
// internally-tagged enum keyed on "cmd" ("subscribe" / "unsubscribe" / "ping")
// with optional "queue" and "job_id" fields. It sends no acknowledgement for
// subscribe/unsubscribe, so commands are written fire-and-forget.
type wsCommand struct {
	Cmd   string `json:"cmd"`
	Queue string `json:"queue,omitempty"`
	JobID string `json:"job_id,omitempty"`
}
