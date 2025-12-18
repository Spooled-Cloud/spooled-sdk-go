package types

import "time"

// JobStatus represents the status of a job.
type JobStatus string

const (
	JobStatusPending    JobStatus = "pending"
	JobStatusScheduled  JobStatus = "scheduled"
	JobStatusProcessing JobStatus = "processing"
	JobStatusCompleted  JobStatus = "completed"
	JobStatusFailed     JobStatus = "failed"
	JobStatusDeadletter JobStatus = "deadletter"
	JobStatusCancelled  JobStatus = "cancelled"
)

// CreateJobRequest is the request to create a new job.
type CreateJobRequest struct {
	QueueName         string      `json:"queue_name"`
	Payload           JsonObject  `json:"payload"`
	Priority          *int        `json:"priority,omitempty"`
	MaxRetries        *int        `json:"max_retries,omitempty"`
	TimeoutSeconds    *int        `json:"timeout_seconds,omitempty"`
	ScheduledAt       *time.Time  `json:"scheduled_at,omitempty"`
	ExpiresAt         *time.Time  `json:"expires_at,omitempty"`
	IdempotencyKey    *string     `json:"idempotency_key,omitempty"`
	Tags              *JsonObject `json:"tags,omitempty"`
	ParentJobID       *string     `json:"parent_job_id,omitempty"`
	CompletionWebhook *string     `json:"completion_webhook,omitempty"`
}

// CreateJobResponse is the response from creating a job.
type CreateJobResponse struct {
	ID      string `json:"id"`
	Created bool   `json:"created"`
}

// Job represents a full job object.
type Job struct {
	ID                string      `json:"id"`
	OrganizationID    string      `json:"organization_id"`
	QueueName         string      `json:"queue_name"`
	Status            JobStatus   `json:"status"`
	Payload           JsonObject  `json:"payload"`
	Result            *JsonObject `json:"result,omitempty"`
	RetryCount        int         `json:"retry_count"`
	MaxRetries        int         `json:"max_retries"`
	LastError         *string     `json:"last_error,omitempty"`
	CreatedAt         time.Time   `json:"created_at"`
	ScheduledAt       *time.Time  `json:"scheduled_at,omitempty"`
	StartedAt         *time.Time  `json:"started_at,omitempty"`
	CompletedAt       *time.Time  `json:"completed_at,omitempty"`
	ExpiresAt         *time.Time  `json:"expires_at,omitempty"`
	Priority          int         `json:"priority"`
	Tags              *JsonObject `json:"tags,omitempty"`
	TimeoutSeconds    int         `json:"timeout_seconds"`
	ParentJobID       *string     `json:"parent_job_id,omitempty"`
	CompletionWebhook *string     `json:"completion_webhook,omitempty"`
	AssignedWorkerID  *string     `json:"assigned_worker_id,omitempty"`
	LeaseID           *string     `json:"lease_id,omitempty"`
	LeaseExpiresAt    *time.Time  `json:"lease_expires_at,omitempty"`
	IdempotencyKey    *string     `json:"idempotency_key,omitempty"`
	UpdatedAt         time.Time   `json:"updated_at"`
	WorkflowID        *string     `json:"workflow_id,omitempty"`
	DependencyMode    *string     `json:"dependency_mode,omitempty"`
	DependenciesMet   *bool       `json:"dependencies_met,omitempty"`
}

// JobSummary is a summary of a job.
type JobSummary struct {
	ID          string     `json:"id"`
	QueueName   string     `json:"queue_name"`
	Status      JobStatus  `json:"status"`
	Priority    int        `json:"priority"`
	RetryCount  int        `json:"retry_count"`
	CreatedAt   time.Time  `json:"created_at"`
	ScheduledAt *time.Time `json:"scheduled_at,omitempty"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

// ListJobsParams are parameters for listing jobs.
type ListJobsParams struct {
	QueueName *string    `json:"queue_name,omitempty"`
	Status    *JobStatus `json:"status,omitempty"`
	Limit     *int       `json:"limit,omitempty"`
	Offset    *int       `json:"offset,omitempty"`
	OrderBy   *string    `json:"order_by,omitempty"`
	OrderDir  *string    `json:"order_dir,omitempty"`
}

// JobStats represents job statistics.
type JobStats struct {
	Pending    int `json:"pending"`
	Scheduled  int `json:"scheduled"`
	Processing int `json:"processing"`
	Completed  int `json:"completed"`
	Failed     int `json:"failed"`
	Deadletter int `json:"deadletter"`
	Cancelled  int `json:"cancelled"`
	Total      int `json:"total"`
}

// BatchJobStatus is the status of a job in a batch.
type BatchJobStatus struct {
	ID     string    `json:"id"`
	Status JobStatus `json:"status"`
}

// BoostPriorityRequest is the request to boost a job's priority.
type BoostPriorityRequest struct {
	Priority int `json:"priority"`
}

// BoostPriorityResponse is the response from boosting a job's priority.
type BoostPriorityResponse struct {
	JobID       string `json:"job_id"`
	OldPriority int    `json:"old_priority"`
	NewPriority int    `json:"new_priority"`
}

// ClaimJobsRequest is the request to claim jobs.
type ClaimJobsRequest struct {
	QueueName        string `json:"queue_name"`
	WorkerID         string `json:"worker_id"`
	Limit            *int   `json:"limit,omitempty"`
	LeaseDurationSec *int   `json:"lease_duration_secs,omitempty"`
}

// ClaimJobsResponse is the response from claiming jobs.
type ClaimJobsResponse struct {
	Jobs []ClaimedJob `json:"jobs"`
}

// ClaimedJob is a job that has been claimed by a worker.
type ClaimedJob struct {
	ID             string     `json:"id"`
	QueueName      string     `json:"queue_name"`
	Payload        JsonObject `json:"payload"`
	RetryCount     int        `json:"retry_count"`
	MaxRetries     int        `json:"max_retries"`
	TimeoutSeconds int        `json:"timeout_seconds"`
	LeaseExpiresAt *time.Time `json:"lease_expires_at,omitempty"`
}

// CompleteJobRequest is the request to complete a job.
type CompleteJobRequest struct {
	WorkerID string      `json:"worker_id"`
	Result   *JsonObject `json:"result,omitempty"`
}

// CompleteJobResponse is the response from completing a job.
type CompleteJobResponse struct {
	Success bool `json:"success"`
}

// FailJobRequest is the request to fail a job.
type FailJobRequest struct {
	WorkerID string `json:"worker_id"`
	Error    string `json:"error"`
}

// FailJobResponse is the response from failing a job.
type FailJobResponse struct {
	Success bool `json:"success"`
}

// HeartbeatJobRequest is the request for a job heartbeat.
type HeartbeatJobRequest struct {
	WorkerID         string `json:"worker_id"`
	LeaseDurationSec *int   `json:"lease_duration_secs,omitempty"`
}

// HeartbeatJobResponse is the response from a job heartbeat.
type HeartbeatJobResponse struct {
	Success bool `json:"success"`
}

// BulkEnqueueRequest is the request to bulk enqueue jobs.
type BulkEnqueueRequest struct {
	QueueName             string           `json:"queue_name"`
	Jobs                  []BulkJobItem    `json:"jobs"`
	DefaultPriority       *int             `json:"default_priority,omitempty"`
	DefaultMaxRetries     *int             `json:"default_max_retries,omitempty"`
	DefaultTimeoutSeconds *int             `json:"default_timeout_seconds,omitempty"`
}

// BulkJobItem is an individual job in a bulk enqueue request.
type BulkJobItem struct {
	Payload        JsonObject `json:"payload"`
	Priority       *int       `json:"priority,omitempty"`
	IdempotencyKey *string    `json:"idempotency_key,omitempty"`
	ScheduledAt    *time.Time `json:"scheduled_at,omitempty"`
}

// BulkEnqueueResponse is the response from bulk enqueueing jobs.
type BulkEnqueueResponse struct {
	Succeeded    []BulkJobSuccess `json:"succeeded"`
	Failed       []BulkJobFailure `json:"failed"`
	Total        int              `json:"total"`
	SuccessCount int              `json:"success_count"`
	FailureCount int              `json:"failure_count"`
}

// BulkJobSuccess represents a successfully enqueued job.
type BulkJobSuccess struct {
	Index   int    `json:"index"`
	JobID   string `json:"job_id"`
	Created bool   `json:"created"`
}

// BulkJobFailure represents a failed job in bulk enqueue.
type BulkJobFailure struct {
	Index int    `json:"index"`
	Error string `json:"error"`
}

// DLQ types

// ListDLQParams are parameters for listing DLQ jobs.
type ListDLQParams struct {
	QueueName *string `json:"queue_name,omitempty"`
	Limit     *int    `json:"limit,omitempty"`
	Offset    *int    `json:"offset,omitempty"`
}

// RetryDLQRequest is the request to retry DLQ jobs.
type RetryDLQRequest struct {
	QueueName *string `json:"queue_name,omitempty"`
	JobIDs    []string `json:"job_ids,omitempty"`
}

// RetryDLQResponse is the response from retrying DLQ jobs.
type RetryDLQResponse struct {
	RetriedCount int      `json:"retried_count"`
	RetriedJobs  []string `json:"retried_jobs,omitempty"`
}

// PurgeDLQRequest is the request to purge DLQ jobs.
type PurgeDLQRequest struct {
	QueueName *string `json:"queue_name,omitempty"`
}

// PurgeDLQResponse is the response from purging DLQ jobs.
type PurgeDLQResponse struct {
	PurgedCount int `json:"purged_count"`
}


