package resources

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
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
	ID              string     `json:"id"`
	OrganizationID  string     `json:"organization_id"`
	Name            string     `json:"name"`
	Description     *string    `json:"description,omitempty"`
	CronExpression  string     `json:"cron_expression"`
	Timezone        string     `json:"timezone"`
	QueueName       string     `json:"queue_name"`
	PayloadTemplate any        `json:"payload_template"`
	Priority        int        `json:"priority"`
	MaxRetries      int        `json:"max_retries"`
	TimeoutSeconds  int        `json:"timeout_seconds"`
	IsActive        bool       `json:"is_active"`
	LastRunAt       *time.Time `json:"last_run_at,omitempty"`
	NextRunAt       *time.Time `json:"next_run_at,omitempty"`
	RunCount        int        `json:"run_count"`
	Tags            any        `json:"tags,omitempty"`
	Metadata        any        `json:"metadata,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// ListSchedulesParams are parameters for listing schedules.
//
// GET /schedules honors only `limit` (default 100, max 500) and returns the
// newest schedules first. It ignores queue_name, is_active and offset, which this
// SDK used to send anyway, so a filtered or paged call silently returned every
// schedule (an Offset loop read the same first page forever). QueueName, IsActive
// and Offset are therefore applied client-side, to the newest 500 schedules the
// API returns.
type ListSchedulesParams struct {
	QueueName *string `json:"queue_name,omitempty"`
	IsActive  *bool   `json:"is_active,omitempty"`
	Limit     *int    `json:"limit,omitempty"`
	Offset    *int    `json:"offset,omitempty"`
}

// maxScheduleListLimit is the largest page GET /schedules returns.
const maxScheduleListLimit = 500

// List retrieves schedules, newest first.
//
// With no filter, Limit is sent to the API as is. With QueueName, IsActive or
// Offset the SDK fetches the largest page the API allows (500) and filters it
// locally, then applies Offset and Limit to the filtered list.
func (r *SchedulesResource) List(ctx context.Context, params *ListSchedulesParams) ([]Schedule, error) {
	query := url.Values{}
	clientSide := params != nil && (params.QueueName != nil || params.IsActive != nil || params.Offset != nil)
	if params != nil {
		switch {
		case clientSide:
			query.Set("limit", strconv.Itoa(maxScheduleListLimit))
		case params.Limit != nil:
			query.Set("limit", strconv.Itoa(*params.Limit))
		}
	}

	var result []Schedule
	if err := r.base.GetWithQuery(ctx, "/api/v1/schedules", query, &result); err != nil {
		return nil, err
	}
	if !clientSide {
		return result, nil
	}

	filtered := make([]Schedule, 0, len(result))
	for _, sched := range result {
		if params.QueueName != nil && sched.QueueName != *params.QueueName {
			continue
		}
		if params.IsActive != nil && sched.IsActive != *params.IsActive {
			continue
		}
		filtered = append(filtered, sched)
	}
	return paginate(filtered, params.Offset, params.Limit), nil
}

// CreateScheduleRequest is the request to create a schedule.
type CreateScheduleRequest struct {
	Name            string  `json:"name"`
	Description     *string `json:"description,omitempty"`
	CronExpression  string  `json:"cron_expression"`
	Timezone        *string `json:"timezone,omitempty"`
	QueueName       string  `json:"queue_name"`
	PayloadTemplate any     `json:"payload_template"`
	Priority        *int    `json:"priority,omitempty"`
	MaxRetries      *int    `json:"max_retries,omitempty"`
	TimeoutSeconds  *int    `json:"timeout_seconds,omitempty"`
	Tags            any     `json:"tags,omitempty"`
	Metadata        any     `json:"metadata,omitempty"`
}

// createScheduleResponse is POST /schedules. The body is id/name/cron/next_run
// only — unmarshalling it into Schedule left IsActive false (zero value) even
// though the row is inserted active, and dropped queue/timezone/payload.
type createScheduleResponse struct {
	ID             string     `json:"id"`
	Name           string     `json:"name"`
	CronExpression string     `json:"cron_expression"`
	NextRunAt      *time.Time `json:"next_run_at,omitempty"`
}

// Create creates a new schedule.
func (r *SchedulesResource) Create(ctx context.Context, req *CreateScheduleRequest) (*Schedule, error) {
	var created createScheduleResponse
	if err := r.base.Post(ctx, "/api/v1/schedules", req, &created); err != nil {
		return nil, err
	}
	return scheduleFromCreate(created, req), nil
}

func scheduleFromCreate(created createScheduleResponse, req *CreateScheduleRequest) *Schedule {
	s := &Schedule{
		ID:             created.ID,
		Name:           created.Name,
		CronExpression: created.CronExpression,
		NextRunAt:      created.NextRunAt,
		Timezone:       "UTC",
		IsActive:       true,
	}
	if req == nil {
		return s
	}
	s.QueueName = req.QueueName
	s.PayloadTemplate = req.PayloadTemplate
	s.Description = req.Description
	s.Tags = req.Tags
	s.Metadata = req.Metadata
	if req.Timezone != nil && *req.Timezone != "" {
		s.Timezone = *req.Timezone
	}
	if req.Priority != nil {
		s.Priority = *req.Priority
	}
	if req.MaxRetries != nil {
		s.MaxRetries = *req.MaxRetries
	}
	if req.TimeoutSeconds != nil {
		s.TimeoutSeconds = *req.TimeoutSeconds
	}
	return s
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
	Name           *string `json:"name,omitempty"`
	Description    *string `json:"description,omitempty"`
	CronExpression *string `json:"cron_expression,omitempty"`
	Timezone       *string `json:"timezone,omitempty"`
	// QueueName is not part of the API's update request, so the server ignores
	// it: a schedule's queue cannot be changed. Delete and recreate the schedule
	// to move it to another queue.
	//
	// Deprecated: has no effect.
	QueueName       *string `json:"queue_name,omitempty"`
	PayloadTemplate any     `json:"payload_template,omitempty"`
	Priority        *int    `json:"priority,omitempty"`
	MaxRetries      *int    `json:"max_retries,omitempty"`
	TimeoutSeconds  *int    `json:"timeout_seconds,omitempty"`
	IsActive        *bool   `json:"is_active,omitempty"`
	Tags            any     `json:"tags,omitempty"`
	Metadata        any     `json:"metadata,omitempty"`
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
