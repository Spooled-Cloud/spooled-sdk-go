package types

import "time"

// Schedule represents a scheduled job.
type Schedule struct {
	ID              string      `json:"id"`
	OrganizationID  string      `json:"organization_id"`
	Name            string      `json:"name"`
	Description     *string     `json:"description,omitempty"`
	CronExpression  string      `json:"cron_expression"`
	Timezone        string      `json:"timezone"`
	QueueName       string      `json:"queue_name"`
	PayloadTemplate JsonObject  `json:"payload_template"`
	Priority        int         `json:"priority"`
	MaxRetries      int         `json:"max_retries"`
	TimeoutSeconds  int         `json:"timeout_seconds"`
	IsActive        bool        `json:"is_active"`
	LastRunAt       *time.Time  `json:"last_run_at,omitempty"`
	NextRunAt       *time.Time  `json:"next_run_at,omitempty"`
	RunCount        int         `json:"run_count"`
	Tags            *JsonObject `json:"tags,omitempty"`
	Metadata        *JsonObject `json:"metadata,omitempty"`
	CreatedAt       time.Time   `json:"created_at"`
	UpdatedAt       time.Time   `json:"updated_at"`
}

// CreateScheduleRequest is the request to create a schedule.
type CreateScheduleRequest struct {
	Name            string      `json:"name"`
	Description     *string     `json:"description,omitempty"`
	CronExpression  string      `json:"cron_expression"`
	Timezone        *string     `json:"timezone,omitempty"`
	QueueName       string      `json:"queue_name"`
	PayloadTemplate JsonObject  `json:"payload_template"`
	Priority        *int        `json:"priority,omitempty"`
	MaxRetries      *int        `json:"max_retries,omitempty"`
	TimeoutSeconds  *int        `json:"timeout_seconds,omitempty"`
	Tags            *JsonObject `json:"tags,omitempty"`
	Metadata        *JsonObject `json:"metadata,omitempty"`
}

// CreateScheduleResponse is the response from creating a schedule.
type CreateScheduleResponse struct {
	ID             string     `json:"id"`
	Name           string     `json:"name"`
	CronExpression string     `json:"cron_expression"`
	NextRunAt      *time.Time `json:"next_run_at,omitempty"`
}

// UpdateScheduleRequest is the request to update a schedule.
type UpdateScheduleRequest struct {
	Name            *string     `json:"name,omitempty"`
	Description     *string     `json:"description,omitempty"`
	CronExpression  *string     `json:"cron_expression,omitempty"`
	Timezone        *string     `json:"timezone,omitempty"`
	QueueName       *string     `json:"queue_name,omitempty"`
	PayloadTemplate *JsonObject `json:"payload_template,omitempty"`
	Priority        *int        `json:"priority,omitempty"`
	MaxRetries      *int        `json:"max_retries,omitempty"`
	TimeoutSeconds  *int        `json:"timeout_seconds,omitempty"`
	IsActive        *bool       `json:"is_active,omitempty"`
	Tags            *JsonObject `json:"tags,omitempty"`
	Metadata        *JsonObject `json:"metadata,omitempty"`
}

// TriggerScheduleResponse is the response from triggering a schedule.
type TriggerScheduleResponse struct {
	JobID       string    `json:"job_id"`
	TriggeredAt time.Time `json:"triggered_at"`
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

// ListSchedulesParams are parameters for listing schedules.
type ListSchedulesParams struct {
	QueueName *string `json:"queue_name,omitempty"`
	IsActive  *bool   `json:"is_active,omitempty"`
	Limit     *int    `json:"limit,omitempty"`
	Offset    *int    `json:"offset,omitempty"`
}


