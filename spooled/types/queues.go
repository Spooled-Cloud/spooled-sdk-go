package types

import "time"

// QueueConfig represents queue configuration.
type QueueConfig struct {
	ID             string     `json:"id"`
	OrganizationID string     `json:"organization_id"`
	QueueName      string     `json:"queue_name"`
	MaxRetries     int        `json:"max_retries"`
	DefaultTimeout int        `json:"default_timeout"`
	RateLimit      *int       `json:"rate_limit,omitempty"`
	Enabled        bool       `json:"enabled"`
	Settings       JsonObject `json:"settings"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// QueueConfigSummary is a summary of queue configuration.
type QueueConfigSummary struct {
	QueueName      string `json:"queue_name"`
	MaxRetries     int    `json:"max_retries"`
	DefaultTimeout int    `json:"default_timeout"`
	RateLimit      *int   `json:"rate_limit,omitempty"`
	Enabled        bool   `json:"enabled"`
}

// UpdateQueueConfigRequest is the request to update queue configuration.
type UpdateQueueConfigRequest struct {
	MaxRetries     *int  `json:"max_retries,omitempty"`
	DefaultTimeout *int  `json:"default_timeout,omitempty"`
	RateLimit      *int  `json:"rate_limit,omitempty"`
	Enabled        *bool `json:"enabled,omitempty"`
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

// ResumeQueueResponse is the response from resuming a queue.
type ResumeQueueResponse struct {
	QueueName          string `json:"queue_name"`
	Resumed            bool   `json:"resumed"`
	PausedDurationSecs int    `json:"paused_duration_secs"`
}
