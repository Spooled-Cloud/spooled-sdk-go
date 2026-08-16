package types

import "time"

// WorkerStatus represents the status of a worker.
type WorkerStatus string

const (
	WorkerStatusHealthy  WorkerStatus = "healthy"
	WorkerStatusDegraded WorkerStatus = "degraded"
	WorkerStatusOffline  WorkerStatus = "offline"
	WorkerStatusDraining WorkerStatus = "draining"
)

// Worker represents a worker.
type Worker struct {
	ID             string       `json:"id"`
	OrganizationID string       `json:"organization_id"`
	QueueName      string       `json:"queue_name"`
	Hostname       string       `json:"hostname"`
	WorkerType     *string      `json:"worker_type,omitempty"`
	MaxConcurrency int          `json:"max_concurrency"`
	CurrentJobs    int          `json:"current_jobs"`
	Status         WorkerStatus `json:"status"`
	LastHeartbeat  time.Time    `json:"last_heartbeat"`
	Metadata       JsonObject   `json:"metadata"`
	Version        *string      `json:"version,omitempty"`
	RegisteredAt   time.Time    `json:"registered_at"`
}

// WorkerSummary is a summary of a worker.
type WorkerSummary struct {
	ID             string       `json:"id"`
	QueueName      string       `json:"queue_name"`
	Hostname       string       `json:"hostname"`
	Status         WorkerStatus `json:"status"`
	CurrentJobs    int          `json:"current_jobs"`
	MaxConcurrency int          `json:"max_concurrency"`
	LastHeartbeat  time.Time    `json:"last_heartbeat"`
}

// RegisterWorkerRequest is the request to register a worker.
type RegisterWorkerRequest struct {
	// WorkerID is an optional stable worker identity: 1-128 characters from
	// [A-Za-z0-9._-]. Supplying the same value across restarts makes
	// registration an upsert, so the process reuses one worker row. Leave it
	// nil and the server mints a UUID instead, which means every restart
	// leaves the previous row counting against the plan worker cap until the
	// stale-worker reaper clears it (roughly two minutes) - enough for a
	// crash-looping worker on a tight plan to 429 itself out of registering.
	// Re-registering an ID this organization already owns is not charged
	// against the cap; an ID owned by a different organization returns 409.
	WorkerID       *string     `json:"worker_id,omitempty"`
	QueueName      string      `json:"queue_name"`
	Hostname       string      `json:"hostname"`
	WorkerType     *string     `json:"worker_type,omitempty"`
	MaxConcurrency *int        `json:"max_concurrency,omitempty"`
	Metadata       *JsonObject `json:"metadata,omitempty"`
	Version        *string     `json:"version,omitempty"`
}

// RegisterWorkerResponse is the response from registering a worker.
type RegisterWorkerResponse struct {
	ID                   string `json:"id"`
	QueueName            string `json:"queue_name"`
	LeaseDurationSecs    int    `json:"lease_duration_secs"`
	HeartbeatIntervalSec int    `json:"heartbeat_interval_secs"`
}

// WorkerHeartbeatRequest is the request for a worker heartbeat.
type WorkerHeartbeatRequest struct {
	CurrentJobs int         `json:"current_jobs"`
	Status      *string     `json:"status,omitempty"`
	Metadata    *JsonObject `json:"metadata,omitempty"`
}
