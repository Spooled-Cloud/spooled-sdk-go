package resources

import (
	"context"
	"fmt"
	"time"

	"github.com/spooled-cloud/spooled-sdk-go/internal/httpx"
)

// QueuesResource provides access to queue operations.
type QueuesResource struct {
	base *Base
}

// NewQueuesResource creates a new QueuesResource.
func NewQueuesResource(transport *httpx.Transport) *QueuesResource {
	return &QueuesResource{base: NewBase(transport)}
}

// QueueListItem represents a queue in list responses (simplified).
type QueueListItem struct {
	QueueName      string `json:"queue_name"`
	MaxRetries     int    `json:"max_retries"`
	DefaultTimeout int    `json:"default_timeout"`
	RateLimit      *int   `json:"rate_limit,omitempty"`
	Enabled        bool   `json:"enabled"`
}

// QueueConfig represents full queue configuration (from Get).
type QueueConfig struct {
	ID             string         `json:"id"`
	OrganizationID string         `json:"organization_id"`
	QueueName      string         `json:"queue_name"`
	MaxRetries     int            `json:"max_retries"`
	DefaultTimeout int            `json:"default_timeout"`
	RateLimit      *int           `json:"rate_limit,omitempty"`
	Enabled        bool           `json:"enabled"`
	Settings       map[string]any `json:"settings"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
}

// List retrieves all queue configurations.
func (r *QueuesResource) List(ctx context.Context) ([]QueueListItem, error) {
	var result []QueueListItem
	if err := r.base.Get(ctx, "/api/v1/queues", &result); err != nil {
		return nil, err
	}
	return result, nil
}

// Get retrieves a specific queue configuration.
func (r *QueuesResource) Get(ctx context.Context, name string) (*QueueConfig, error) {
	var result QueueConfig
	if err := r.base.Get(ctx, fmt.Sprintf("/api/v1/queues/%s", name), &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// UpdateQueueConfigRequest is the request to update queue configuration.
type UpdateQueueConfigRequest struct {
	MaxRetries     *int  `json:"max_retries,omitempty"`
	DefaultTimeout *int  `json:"default_timeout,omitempty"`
	RateLimit      *int  `json:"rate_limit,omitempty"`
	Enabled        *bool `json:"enabled,omitempty"`
}

// UpdateConfig updates a queue's configuration.
func (r *QueuesResource) UpdateConfig(ctx context.Context, name string, req *UpdateQueueConfigRequest) (*QueueConfig, error) {
	var result QueueConfig
	if err := r.base.Put(ctx, fmt.Sprintf("/api/v1/queues/%s", name), req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// QueueStats represents queue statistics.
type QueueStats struct {
	QueueName           string `json:"queue_name"`
	PendingJobs         int    `json:"pending_jobs"`
	ProcessingJobs      int    `json:"processing_jobs"`
	CompletedJobs24h    int    `json:"completed_jobs_24h"`
	FailedJobs24h       int    `json:"failed_jobs_24h"`
	AvgProcessingTimeMs *int   `json:"avg_processing_time_ms,omitempty"`
	MaxJobAgeSeconds    *int   `json:"max_job_age_seconds,omitempty"`
	ActiveWorkers       int    `json:"active_workers"`
}

// GetStats retrieves statistics for a queue.
func (r *QueuesResource) GetStats(ctx context.Context, name string) (*QueueStats, error) {
	var result QueueStats
	if err := r.base.Get(ctx, fmt.Sprintf("/api/v1/queues/%s/stats", name), &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// PauseQueueRequest is the request to pause a queue.
type PauseQueueRequest struct {
	Reason *string `json:"reason,omitempty"`
}

// PauseQueueResponse is the response from pausing a queue.
type PauseQueueResponse struct {
	QueueName string    `json:"queue_name"`
	Paused    bool      `json:"paused"`
	PausedAt  time.Time `json:"paused_at"`
	Reason    *string   `json:"reason,omitempty"`
}

// Pause pauses a queue.
func (r *QueuesResource) Pause(ctx context.Context, name string, req *PauseQueueRequest) (*PauseQueueResponse, error) {
	var result PauseQueueResponse
	body := req
	if body == nil {
		body = &PauseQueueRequest{}
	}
	if err := r.base.Post(ctx, fmt.Sprintf("/api/v1/queues/%s/pause", name), body, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// ResumeQueueResponse is the response from resuming a queue.
type ResumeQueueResponse struct {
	QueueName          string `json:"queue_name"`
	Resumed            bool   `json:"resumed"`
	PausedDurationSecs int    `json:"paused_duration_secs"`
}

// Resume resumes a paused queue.
func (r *QueuesResource) Resume(ctx context.Context, name string) (*ResumeQueueResponse, error) {
	var result ResumeQueueResponse
	if err := r.base.Post(ctx, fmt.Sprintf("/api/v1/queues/%s/resume", name), nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// Delete deletes a queue configuration.
func (r *QueuesResource) Delete(ctx context.Context, name string) error {
	return r.base.Delete(ctx, fmt.Sprintf("/api/v1/queues/%s", name))
}

