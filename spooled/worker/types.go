// Package worker provides job processing runtime for Spooled queues.
package worker

import (
	"context"
	"time"
)

// State represents the worker state.
type State string

const (
	StateIdle     State = "idle"
	StateStarting State = "starting"
	StateRunning  State = "running"
	StateStopping State = "stopping"
	StateStopped  State = "stopped"
	StateError    State = "error"
)

// Options configures a worker.
type Options struct {
	// QueueName is the name of the queue to process
	QueueName string
	// Hostname is the worker hostname (default: auto-detected)
	Hostname string
	// WorkerType is an identifier for this worker type
	WorkerType string
	// Concurrency is the maximum concurrent jobs (1-100, default: 5)
	Concurrency int
	// PollInterval is the polling interval (default: 1s)
	PollInterval time.Duration
	// LeaseDuration is the job lease duration in seconds (5-3600, default: 30)
	LeaseDuration int
	// HeartbeatFraction is the heartbeat interval as a fraction of lease duration (default: 0.5)
	HeartbeatFraction float64
	// ShutdownTimeout is the graceful shutdown timeout (default: 30s)
	ShutdownTimeout time.Duration
	// Version is the worker version string
	Version string
	// Metadata is additional worker metadata
	Metadata map[string]string
	// Debug enables debug logging
	Debug bool
	// Logger is a custom logger function
	Logger func(msg string, args ...any)
}

// DefaultOptions returns options with sensible defaults.
func DefaultOptions() Options {
	return Options{
		Concurrency:       5,
		PollInterval:      1 * time.Second,
		LeaseDuration:     30,
		HeartbeatFraction: 0.5,
		ShutdownTimeout:   30 * time.Second,
		WorkerType:        "go",
		Version:           "0.1.0",
	}
}

// JobContext provides context and utilities for job handlers.
type JobContext struct {
	// Context is the Go context with cancellation
	Context context.Context
	// JobID is the unique job identifier
	JobID string
	// QueueName is the queue this job belongs to
	QueueName string
	// Payload is the job payload data
	Payload map[string]any
	// RetryCount is the current retry attempt number
	RetryCount int
	// MaxRetries is the maximum number of retries
	MaxRetries int
	// Progress reports job progress (0-100)
	Progress func(percent float64, message string) error
	// Log logs a message at the specified level
	Log func(level string, message string, meta map[string]any)

	// Internal fields
	workerID string
	worker   *Worker
}

// JobHandler is a function that processes a job.
// Return an error to fail the job, or nil/result to complete it.
type JobHandler func(ctx *JobContext) (map[string]any, error)

// Event types for worker events.
type EventType string

const (
	EventWorkerStarted    EventType = "worker:started"
	EventWorkerStopped    EventType = "worker:stopped"
	EventWorkerError      EventType = "worker:error"
	EventJobClaimed       EventType = "job:claimed"
	EventJobStarted       EventType = "job:started"
	EventJobCompleted     EventType = "job:completed"
	EventJobFailed        EventType = "job:failed"
	EventJobProgress      EventType = "job:progress"
	EventJobHeartbeat     EventType = "job:heartbeat"
	EventWorkerHeartbeat  EventType = "worker:heartbeat"
)

// Event is emitted by the worker during processing.
type Event struct {
	Type      EventType
	Timestamp time.Time
	Data      any
}

// WorkerStartedData is emitted when the worker starts.
type WorkerStartedData struct {
	WorkerID  string
	QueueName string
}

// WorkerStoppedData is emitted when the worker stops.
type WorkerStoppedData struct {
	WorkerID string
	Reason   string
}

// WorkerErrorData is emitted on worker errors.
type WorkerErrorData struct {
	Error error
}

// JobClaimedData is emitted when a job is claimed.
type JobClaimedData struct {
	JobID     string
	QueueName string
}

// JobStartedData is emitted when job processing starts.
type JobStartedData struct {
	JobID     string
	QueueName string
}

// JobCompletedData is emitted when a job completes.
type JobCompletedData struct {
	JobID     string
	QueueName string
	Result    map[string]any
	Duration  time.Duration
}

// JobFailedData is emitted when a job fails.
type JobFailedData struct {
	JobID     string
	QueueName string
	Error     error
	Duration  time.Duration
	WillRetry bool
}

// JobProgressData is emitted when job progress is updated.
type JobProgressData struct {
	JobID    string
	Percent  float64
	Message  string
}

// EventHandler is a callback for worker events.
type EventHandler func(event Event)


