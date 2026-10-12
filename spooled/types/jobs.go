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
//
// Payload and Tags are `any`, not `map[string]any`: the API stores both as
// `serde_json::Value`, so a JSON array, string, number or boolean is a valid
// payload, and a map type could neither send nor round-trip one.
type CreateJobRequest struct {
	QueueName         string     `json:"queue_name"`
	Payload           any        `json:"payload"`
	Priority          *int       `json:"priority,omitempty"`
	MaxRetries        *int       `json:"max_retries,omitempty"`
	TimeoutSeconds    *int       `json:"timeout_seconds,omitempty"`
	ScheduledAt       *time.Time `json:"scheduled_at,omitempty"`
	ExpiresAt         *time.Time `json:"expires_at,omitempty"`
	IdempotencyKey    *string    `json:"idempotency_key,omitempty"`
	Tags              any        `json:"tags,omitempty"`
	ParentJobID       *string    `json:"parent_job_id,omitempty"`
	CompletionWebhook *string    `json:"completion_webhook,omitempty"`
}

// CreateJobResponse is the response from creating a job.
type CreateJobResponse struct {
	ID      string `json:"id"`
	Created bool   `json:"created"`
}

// Job represents a full job object.
type Job struct {
	ID                string     `json:"id"`
	OrganizationID    string     `json:"organization_id"`
	QueueName         string     `json:"queue_name"`
	Status            JobStatus  `json:"status"`
	Payload           any        `json:"payload"`
	Result            any        `json:"result,omitempty"`
	RetryCount        int        `json:"retry_count"`
	MaxRetries        int        `json:"max_retries"`
	LastError         *string    `json:"last_error,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	ScheduledAt       *time.Time `json:"scheduled_at,omitempty"`
	StartedAt         *time.Time `json:"started_at,omitempty"`
	CompletedAt       *time.Time `json:"completed_at,omitempty"`
	ExpiresAt         *time.Time `json:"expires_at,omitempty"`
	Priority          int        `json:"priority"`
	Tags              any        `json:"tags,omitempty"`
	TimeoutSeconds    int        `json:"timeout_seconds"`
	ParentJobID       *string    `json:"parent_job_id,omitempty"`
	CompletionWebhook *string    `json:"completion_webhook,omitempty"`
	AssignedWorkerID  *string    `json:"assigned_worker_id,omitempty"`
	LeaseID           *string    `json:"lease_id,omitempty"`
	LeaseExpiresAt    *time.Time `json:"lease_expires_at,omitempty"`
	IdempotencyKey    *string    `json:"idempotency_key,omitempty"`
	UpdatedAt         time.Time  `json:"updated_at"`
	WorkflowID        *string    `json:"workflow_id,omitempty"`
	DependencyMode    *string    `json:"dependency_mode,omitempty"`
	DependenciesMet   *bool      `json:"dependencies_met,omitempty"`
}

// JobSummary is a summary of a job.
type JobSummary struct {
	ID          string     `json:"id"`
	QueueName   string     `json:"queue_name"`
	Status      JobStatus  `json:"status"`
	Priority    int        `json:"priority"`
	RetryCount  int        `json:"attempt"`
	MaxRetries  int        `json:"max_retries"`
	JobType     string     `json:"job_type,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	ScheduledAt *time.Time `json:"scheduled_at,omitempty"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

// ListJobsParams are parameters for listing jobs. See
// resources.ListJobsParams, which sends them.
type ListJobsParams struct {
	QueueName *string    `json:"queue_name,omitempty"`
	Status    *JobStatus `json:"status,omitempty"`
	Tag       *string    `json:"tag,omitempty"` // Filter by a single tag
	Limit     *int       `json:"limit,omitempty"`
	Offset    *int       `json:"offset,omitempty"`
	OrderBy   *string    `json:"order_by,omitempty"`
	OrderDir  *string    `json:"order_dir,omitempty"`
	// Statuses keeps jobs in any of these statuses (one comma-separated
	// status parameter on the wire).
	Statuses []JobStatus `json:"statuses,omitempty"`
	// Q matches a job whose ID starts with it, or whose queue name or payload
	// job_type contains it (case-insensitive).
	Q *string `json:"q,omitempty"`
	// JobType keeps jobs whose payload job_type is exactly this value.
	JobType *string `json:"job_type,omitempty"`
	// CreatedAfter keeps jobs created at or after this instant (RFC 3339).
	CreatedAfter *time.Time `json:"created_after,omitempty"`
	// CreatedBefore keeps jobs created before this instant (RFC 3339).
	CreatedBefore *time.Time `json:"created_before,omitempty"`
}

// JobHistoryEvent is one recorded lifecycle event of a job
// (GET /jobs/{id}/history).
type JobHistoryEvent struct {
	ID    string `json:"id"`
	JobID string `json:"job_id"`
	// EventType names the event; the API can add event types.
	EventType string    `json:"event_type"`
	WorkerID  *string   `json:"worker_id,omitempty"`
	Details   any       `json:"details,omitempty"`
	CreatedAt time.Time `json:"created_at"`
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
// GET /jobs/status sends retry_count, not attempt, and has no max_retries.
type BatchJobStatus struct {
	ID          string     `json:"id"`
	Status      JobStatus  `json:"status"`
	QueueName   string     `json:"queue_name"`
	RetryCount  int        `json:"retry_count"`
	CreatedAt   time.Time  `json:"created_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
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
	ID        string `json:"id"`
	QueueName string `json:"queue_name"`
	// Any JSON, matching resources.ClaimedJob and the API's serde_json::Value.
	Payload        any        `json:"payload"`
	RetryCount     int        `json:"retry_count"`
	MaxRetries     int        `json:"max_retries"`
	TimeoutSeconds int        `json:"timeout_seconds"`
	LeaseExpiresAt *time.Time `json:"lease_expires_at,omitempty"`
	// LeaseID is the lease fencing token for this claim. Complete, fail and
	// heartbeat require it.
	LeaseID *string `json:"lease_id,omitempty"`
}

// CompleteJobRequest is the request to complete a job.
type CompleteJobRequest struct {
	WorkerID string `json:"worker_id"`
	Result   any    `json:"result,omitempty"`
	// LeaseID is the claimed job's lease fencing token. It is required: the
	// API answers a complete without it with 422 VALIDATION_ERROR.
	LeaseID *string `json:"lease_id,omitempty"`
}

// CompleteJobResponse is the response from completing a job.
type CompleteJobResponse struct {
	Success bool `json:"success"`
}

// FailJobRequest is the request to fail a job.
type FailJobRequest struct {
	WorkerID string `json:"worker_id"`
	Error    string `json:"error"`
	// LeaseID is the claimed job's lease fencing token. It is required: the
	// API answers a fail without it with 422 VALIDATION_ERROR.
	LeaseID *string `json:"lease_id,omitempty"`
}

// FailJobResponse is the response from failing a job.
type FailJobResponse struct {
	Success bool `json:"success"`
}

// HeartbeatJobRequest is the request for a job heartbeat.
type HeartbeatJobRequest struct {
	WorkerID         string `json:"worker_id"`
	LeaseDurationSec *int   `json:"lease_duration_secs,omitempty"`
	// LeaseID is the claimed job's lease fencing token. It is required: the
	// API answers a heartbeat without it with 422 VALIDATION_ERROR.
	LeaseID *string `json:"lease_id,omitempty"`
}

// HeartbeatJobResponse is the response from a job heartbeat.
type HeartbeatJobResponse struct {
	Success bool `json:"success"`
}

// BulkEnqueueRequest is the request to bulk enqueue jobs.
type BulkEnqueueRequest struct {
	QueueName             string        `json:"queue_name"`
	Jobs                  []BulkJobItem `json:"jobs"`
	DefaultPriority       *int          `json:"default_priority,omitempty"`
	DefaultMaxRetries     *int          `json:"default_max_retries,omitempty"`
	DefaultTimeoutSeconds *int          `json:"default_timeout_seconds,omitempty"`
}

// BulkJobItem is an individual job in a bulk enqueue request.
//
// Payload is `any` for the same reason as CreateJobRequest.Payload: the API
// field is `serde_json::Value`, not an object.
type BulkJobItem struct {
	Payload        any        `json:"payload"`
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
	QueueName *string  `json:"queue_name,omitempty"`
	JobIDs    []string `json:"job_ids,omitempty"`
}

// RetryDLQResponse is the response from retrying DLQ jobs.
type RetryDLQResponse struct {
	RetriedCount int      `json:"retried_count"`
	RetriedJobs  []string `json:"retried_jobs,omitempty"`
}

// PurgeDLQRequest is the request to purge DLQ jobs.
type PurgeDLQRequest struct {
	QueueName *string    `json:"queue_name,omitempty"`
	OlderThan *time.Time `json:"older_than,omitempty"`
	Limit     *int       `json:"limit,omitempty"`
	Confirm   bool       `json:"confirm"`
}

// PurgeDLQResponse is the response from purging DLQ jobs.
type PurgeDLQResponse struct {
	PurgedCount int `json:"purged_count"`
}
