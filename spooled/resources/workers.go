package resources

import (
	"context"
	"fmt"
	"time"

	"github.com/spooled-cloud/spooled-sdk-go/internal/httpx"
)

// WorkersResource provides access to worker operations.
type WorkersResource struct {
	base *Base
}

// NewWorkersResource creates a new WorkersResource.
func NewWorkersResource(transport *httpx.Transport) *WorkersResource {
	return &WorkersResource{base: NewBase(transport)}
}

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
	ID             string         `json:"id"`
	OrganizationID string         `json:"organization_id"`
	QueueName      string         `json:"queue_name"`
	Hostname       string         `json:"hostname"`
	WorkerType     *string        `json:"worker_type,omitempty"`
	MaxConcurrency int            `json:"max_concurrency"`
	CurrentJobs    int            `json:"current_jobs"`
	Status         WorkerStatus   `json:"status"`
	LastHeartbeat  time.Time      `json:"last_heartbeat"`
	Metadata       map[string]any `json:"metadata"`
	Version        *string        `json:"version,omitempty"`
	RegisteredAt   time.Time      `json:"registered_at"`
}

// List retrieves all registered workers.
func (r *WorkersResource) List(ctx context.Context) ([]Worker, error) {
	var result []Worker
	if err := r.base.Get(ctx, "/api/v1/workers", &result); err != nil {
		return nil, err
	}
	return result, nil
}

// Get retrieves a specific worker.
func (r *WorkersResource) Get(ctx context.Context, id string) (*Worker, error) {
	var result Worker
	if err := r.base.Get(ctx, fmt.Sprintf("/api/v1/workers/%s", id), &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// RegisterWorkerRequest is the request to register a worker.
type RegisterWorkerRequest struct {
	QueueName      string         `json:"queue_name"`
	Hostname       string         `json:"hostname"`
	WorkerType     *string        `json:"worker_type,omitempty"`
	MaxConcurrency *int           `json:"max_concurrency,omitempty"`
	Metadata       map[string]any `json:"metadata,omitempty"`
	Version        *string        `json:"version,omitempty"`
}

// RegisterWorkerResponse is the response from registering a worker.
type RegisterWorkerResponse struct {
	ID                   string `json:"id"`
	QueueName            string `json:"queue_name"`
	LeaseDurationSecs    int    `json:"lease_duration_secs"`
	HeartbeatIntervalSec int    `json:"heartbeat_interval_secs"`
}

// Register registers a new worker.
func (r *WorkersResource) Register(ctx context.Context, req *RegisterWorkerRequest) (*RegisterWorkerResponse, error) {
	var result RegisterWorkerResponse
	if err := r.base.Post(ctx, "/api/v1/workers/register", req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// WorkerHeartbeatRequest is the request for a worker heartbeat.
type WorkerHeartbeatRequest struct {
	CurrentJobs int            `json:"current_jobs"`
	Status      *string        `json:"status,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

// Heartbeat sends a heartbeat for a worker.
func (r *WorkersResource) Heartbeat(ctx context.Context, id string, req *WorkerHeartbeatRequest) error {
	return r.base.Post(ctx, fmt.Sprintf("/api/v1/workers/%s/heartbeat", id), req, nil)
}

// Deregister removes a worker registration.
func (r *WorkersResource) Deregister(ctx context.Context, id string) error {
	return r.base.Delete(ctx, fmt.Sprintf("/api/v1/workers/%s", id))
}


