package resources

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/spooled-cloud/spooled-sdk-go/internal/httpx"
)

// SchedulesResource provides access to schedule operations.
type SchedulesResource struct {
	base *Base
}

// NewSchedulesResource creates a new SchedulesResource.
func NewSchedulesResource(transport *httpx.Transport) *SchedulesResource {
	return &SchedulesResource{base: NewBase(transport)}
}

// Schedule represents a scheduled job.
type Schedule struct {
	ID              string         `json:"id"`
	OrganizationID  string         `json:"organization_id"`
	Name            string         `json:"name"`
	Description     *string        `json:"description,omitempty"`
	CronExpression  string         `json:"cron_expression"`
	Timezone        string         `json:"timezone"`
	QueueName       string         `json:"queue_name"`
	PayloadTemplate map[string]any `json:"payload_template"`
	Priority        int            `json:"priority"`
	MaxRetries      int            `json:"max_retries"`
	TimeoutSeconds  int            `json:"timeout_seconds"`
	IsActive        bool           `json:"is_active"`
	LastRunAt       *time.Time     `json:"last_run_at,omitempty"`
	NextRunAt       *time.Time     `json:"next_run_at,omitempty"`
	RunCount        int            `json:"run_count"`
	Tags            map[string]any `json:"tags,omitempty"`
	Metadata        map[string]any `json:"metadata,omitempty"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
}

// ListSchedulesParams are parameters for listing schedules.
type ListSchedulesParams struct {
	QueueName *string `json:"queue_name,omitempty"`
	IsActive  *bool   `json:"is_active,omitempty"`
	Limit     *int    `json:"limit,omitempty"`
	Offset    *int    `json:"offset,omitempty"`
}

// List retrieves all schedules.
func (r *SchedulesResource) List(ctx context.Context, params *ListSchedulesParams) ([]Schedule, error) {
	query := url.Values{}
	if params != nil {
		if params.QueueName != nil {
			query.Set("queue_name", *params.QueueName)
		}
		if params.IsActive != nil {
			query.Set("is_active", fmt.Sprintf("%t", *params.IsActive))
		}
		AddPaginationParams(query, params.Limit, params.Offset)
	}

	var result []Schedule
	if err := r.base.GetWithQuery(ctx, "/api/v1/schedules", query, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// CreateScheduleRequest is the request to create a schedule.
type CreateScheduleRequest struct {
	Name            string         `json:"name"`
	Description     *string        `json:"description,omitempty"`
	CronExpression  string         `json:"cron_expression"`
	Timezone        *string        `json:"timezone,omitempty"`
	QueueName       string         `json:"queue_name"`
	PayloadTemplate map[string]any `json:"payload_template"`
	Priority        *int           `json:"priority,omitempty"`
	MaxRetries      *int           `json:"max_retries,omitempty"`
	TimeoutSeconds  *int           `json:"timeout_seconds,omitempty"`
	Tags            map[string]any `json:"tags,omitempty"`
	Metadata        map[string]any `json:"metadata,omitempty"`
}

// Create creates a new schedule.
func (r *SchedulesResource) Create(ctx context.Context, req *CreateScheduleRequest) (*Schedule, error) {
	var result Schedule
	if err := r.base.Post(ctx, "/api/v1/schedules", req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// Get retrieves a specific schedule.
func (r *SchedulesResource) Get(ctx context.Context, id string) (*Schedule, error) {
	var result Schedule
	if err := r.base.Get(ctx, fmt.Sprintf("/api/v1/schedules/%s", id), &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// UpdateScheduleRequest is the request to update a schedule.
type UpdateScheduleRequest struct {
	Name            *string        `json:"name,omitempty"`
	Description     *string        `json:"description,omitempty"`
	CronExpression  *string        `json:"cron_expression,omitempty"`
	Timezone        *string        `json:"timezone,omitempty"`
	QueueName       *string        `json:"queue_name,omitempty"`
	PayloadTemplate map[string]any `json:"payload_template,omitempty"`
	Priority        *int           `json:"priority,omitempty"`
	MaxRetries      *int           `json:"max_retries,omitempty"`
	TimeoutSeconds  *int           `json:"timeout_seconds,omitempty"`
	IsActive        *bool          `json:"is_active,omitempty"`
	Tags            map[string]any `json:"tags,omitempty"`
	Metadata        map[string]any `json:"metadata,omitempty"`
}

// Update updates a schedule.
func (r *SchedulesResource) Update(ctx context.Context, id string, req *UpdateScheduleRequest) (*Schedule, error) {
	var result Schedule
	if err := r.base.Put(ctx, fmt.Sprintf("/api/v1/schedules/%s", id), req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// Delete deletes a schedule.
func (r *SchedulesResource) Delete(ctx context.Context, id string) error {
	return r.base.Delete(ctx, fmt.Sprintf("/api/v1/schedules/%s", id))
}

// Pause pauses a schedule.
func (r *SchedulesResource) Pause(ctx context.Context, id string) (*Schedule, error) {
	var result Schedule
	if err := r.base.Post(ctx, fmt.Sprintf("/api/v1/schedules/%s/pause", id), nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// Resume resumes a paused schedule.
func (r *SchedulesResource) Resume(ctx context.Context, id string) (*Schedule, error) {
	var result Schedule
	if err := r.base.Post(ctx, fmt.Sprintf("/api/v1/schedules/%s/resume", id), nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// TriggerScheduleResponse is the response from triggering a schedule.
type TriggerScheduleResponse struct {
	JobID       string    `json:"job_id"`
	TriggeredAt time.Time `json:"triggered_at"`
}

// Trigger manually triggers a schedule.
func (r *SchedulesResource) Trigger(ctx context.Context, id string) (*TriggerScheduleResponse, error) {
	var result TriggerScheduleResponse
	if err := r.base.Post(ctx, fmt.Sprintf("/api/v1/schedules/%s/trigger", id), nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// ScheduleRunStatus represents the status of a schedule run.
type ScheduleRunStatus string

const (
	ScheduleRunStatusPending   ScheduleRunStatus = "pending"
	ScheduleRunStatusRunning   ScheduleRunStatus = "running"
	ScheduleRunStatusCompleted ScheduleRunStatus = "completed"
	ScheduleRunStatusFailed    ScheduleRunStatus = "failed"
)

// ScheduleRun represents a schedule execution run.
type ScheduleRun struct {
	ID           string            `json:"id"`
	ScheduleID   string            `json:"schedule_id"`
	JobID        *string           `json:"job_id,omitempty"`
	Status       ScheduleRunStatus `json:"status"`
	ErrorMessage *string           `json:"error_message,omitempty"`
	StartedAt    time.Time         `json:"started_at"`
	CompletedAt  *time.Time        `json:"completed_at,omitempty"`
}

// History retrieves the run history for a schedule.
func (r *SchedulesResource) History(ctx context.Context, id string, limit *int) ([]ScheduleRun, error) {
	query := url.Values{}
	if limit != nil {
		AddPaginationParams(query, limit, nil)
	}

	var result []ScheduleRun
	if err := r.base.GetWithQuery(ctx, fmt.Sprintf("/api/v1/schedules/%s/history", id), query, &result); err != nil {
		return nil, err
	}
	return result, nil
}


