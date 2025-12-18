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
	ID               string       `json:"id"`
	OrganizationID   string       `json:"organization_id"`
	QueueName        string       `json:"queue_name"`
	Hostname         string       `json:"hostname"`
	WorkerType       *string      `json:"worker_type,omitempty"`
	MaxConcurrency   int          `json:"max_concurrency"`
	CurrentJobs      int          `json:"current_jobs"`
	Status           WorkerStatus `json:"status"`
	LastHeartbeat    time.Time    `json:"last_heartbeat"`
	Metadata         JsonObject   `json:"metadata"`
	Version          *string      `json:"version,omitempty"`
	RegisteredAt     time.Time    `json:"registered_at"`
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


