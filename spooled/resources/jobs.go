package resources

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/spooled-cloud/spooled-sdk-go/internal/httpx"
)

// JobsResource provides access to job operations.
type JobsResource struct {
	base *Base
	dlq  *DLQResource
}

// NewJobsResource creates a new JobsResource.
func NewJobsResource(transport *httpx.Transport) *JobsResource {
	base := NewBase(transport)
	return &JobsResource{
		base: base,
		dlq:  &DLQResource{base: base},
	}
}

// DLQ returns the Dead Letter Queue resource.
func (r *JobsResource) DLQ() *DLQResource {
	return r.dlq
}

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

// Job represents a full job object.
type Job struct {
	ID                string                 `json:"id"`
	OrganizationID    string                 `json:"organization_id"`
	QueueName         string                 `json:"queue_name"`
	Status            JobStatus              `json:"status"`
	Payload           map[string]any         `json:"payload"`
	Result            map[string]any         `json:"result,omitempty"`
	RetryCount        int                    `json:"retry_count"`
	MaxRetries        int                    `json:"max_retries"`
	LastError         *string                `json:"last_error,omitempty"`
	CreatedAt         time.Time              `json:"created_at"`
	ScheduledAt       *time.Time             `json:"scheduled_at,omitempty"`
	StartedAt         *time.Time             `json:"started_at,omitempty"`
	CompletedAt       *time.Time             `json:"completed_at,omitempty"`
	ExpiresAt         *time.Time             `json:"expires_at,omitempty"`
	Priority          int                    `json:"priority"`
	Tags              map[string]any         `json:"tags,omitempty"`
	TimeoutSeconds    int                    `json:"timeout_seconds"`
	ParentJobID       *string                `json:"parent_job_id,omitempty"`
	CompletionWebhook *string                `json:"completion_webhook,omitempty"`
	AssignedWorkerID  *string                `json:"assigned_worker_id,omitempty"`
	LeaseID           *string                `json:"lease_id,omitempty"`
	LeaseExpiresAt    *time.Time             `json:"lease_expires_at,omitempty"`
	IdempotencyKey    *string                `json:"idempotency_key,omitempty"`
	UpdatedAt         time.Time              `json:"updated_at"`
	WorkflowID        *string                `json:"workflow_id,omitempty"`
	DependencyMode    *string                `json:"dependency_mode,omitempty"`
	DependenciesMet   *bool                  `json:"dependencies_met,omitempty"`
}

// CreateJobRequest is the request to create a new job.
type CreateJobRequest struct {
	QueueName         string         `json:"queue_name"`
	Payload           map[string]any `json:"payload"`
	Priority          *int           `json:"priority,omitempty"`
	MaxRetries        *int           `json:"max_retries,omitempty"`
	TimeoutSeconds    *int           `json:"timeout_seconds,omitempty"`
	ScheduledAt       *time.Time     `json:"scheduled_at,omitempty"`
	ExpiresAt         *time.Time     `json:"expires_at,omitempty"`
	IdempotencyKey    *string        `json:"idempotency_key,omitempty"`
	Tags              map[string]any `json:"tags,omitempty"`
	ParentJobID       *string        `json:"parent_job_id,omitempty"`
	CompletionWebhook *string        `json:"completion_webhook,omitempty"`
}

// CreateJobResponse is the response from creating a job.
type CreateJobResponse struct {
	ID      string `json:"id"`
	Created bool   `json:"created"`
}

// Create creates a new job.
func (r *JobsResource) Create(ctx context.Context, req *CreateJobRequest) (*CreateJobResponse, error) {
	var result CreateJobResponse
	if err := r.base.Post(ctx, "/api/v1/jobs", req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// CreateAndGet creates a new job and returns the full job object.
func (r *JobsResource) CreateAndGet(ctx context.Context, req *CreateJobRequest) (*Job, error) {
	resp, err := r.Create(ctx, req)
	if err != nil {
		return nil, err
	}
	return r.Get(ctx, resp.ID)
}

// Get retrieves a job by ID.
func (r *JobsResource) Get(ctx context.Context, id string) (*Job, error) {
	var result Job
	if err := r.base.Get(ctx, fmt.Sprintf("/api/v1/jobs/%s", id), &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// ListJobsParams are parameters for listing jobs.
type ListJobsParams struct {
	QueueName *string    `json:"queue_name,omitempty"`
	Status    *JobStatus `json:"status,omitempty"`
	Limit     *int       `json:"limit,omitempty"`
	Offset    *int       `json:"offset,omitempty"`
}

// List retrieves a list of jobs.
func (r *JobsResource) List(ctx context.Context, params *ListJobsParams) ([]Job, error) {
	query := url.Values{}
	if params != nil {
		if params.QueueName != nil {
			query.Set("queue_name", *params.QueueName)
		}
		if params.Status != nil {
			query.Set("status", string(*params.Status))
		}
		AddPaginationParams(query, params.Limit, params.Offset)
	}

	var result []Job
	if err := r.base.GetWithQuery(ctx, "/api/v1/jobs", query, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// Cancel cancels a job.
func (r *JobsResource) Cancel(ctx context.Context, id string) error {
	return r.base.Delete(ctx, fmt.Sprintf("/api/v1/jobs/%s", id))
}

// Retry retries a failed job.
func (r *JobsResource) Retry(ctx context.Context, id string) (*Job, error) {
	var result Job
	if err := r.base.Post(ctx, fmt.Sprintf("/api/v1/jobs/%s/retry", id), nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
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

// BoostPriority boosts a job's priority.
func (r *JobsResource) BoostPriority(ctx context.Context, id string, req *BoostPriorityRequest) (*BoostPriorityResponse, error) {
	var result BoostPriorityResponse
	// Parity with Node/Python: PUT /jobs/{id}/priority
	if err := r.base.Put(ctx, fmt.Sprintf("/api/v1/jobs/%s/priority", id), req, &result); err != nil {
		return nil, err
	}
	return &result, nil
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

// GetStats retrieves job statistics.
func (r *JobsResource) GetStats(ctx context.Context) (*JobStats, error) {
	var result JobStats
	if err := r.base.Get(ctx, "/api/v1/jobs/stats", &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// BatchJobStatus is the status of a job in a batch.
type BatchJobStatus struct {
	ID     string    `json:"id"`
	Status JobStatus `json:"status"`
}

// BatchStatus retrieves the status of multiple jobs.
func (r *JobsResource) BatchStatus(ctx context.Context, ids []string) ([]BatchJobStatus, error) {
	if len(ids) == 0 {
		return []BatchJobStatus{}, nil
	}
	if len(ids) > 100 {
		return nil, fmt.Errorf("maximum 100 job IDs allowed per request")
	}

	query := url.Values{}
	query.Set("ids", strings.Join(ids, ","))

	var result []BatchJobStatus
	if err := r.base.GetWithQuery(ctx, "/api/v1/jobs/status", query, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// BulkJobItem is an individual job in a bulk enqueue request.
type BulkJobItem struct {
	Payload        map[string]any `json:"payload"`
	Priority       *int           `json:"priority,omitempty"`
	IdempotencyKey *string        `json:"idempotency_key,omitempty"`
	ScheduledAt    *time.Time     `json:"scheduled_at,omitempty"`
}

// BulkEnqueueRequest is the request to bulk enqueue jobs.
type BulkEnqueueRequest struct {
	QueueName             string        `json:"queue_name"`
	Jobs                  []BulkJobItem `json:"jobs"`
	DefaultPriority       *int          `json:"default_priority,omitempty"`
	DefaultMaxRetries     *int          `json:"default_max_retries,omitempty"`
	DefaultTimeoutSeconds *int          `json:"default_timeout_seconds,omitempty"`
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

// BulkEnqueueResponse is the response from bulk enqueueing jobs.
type BulkEnqueueResponse struct {
	Succeeded    []BulkJobSuccess `json:"succeeded"`
	Failed       []BulkJobFailure `json:"failed"`
	Total        int              `json:"total"`
	SuccessCount int              `json:"success_count"`
	FailureCount int              `json:"failure_count"`
}

// BulkEnqueue bulk enqueues multiple jobs.
func (r *JobsResource) BulkEnqueue(ctx context.Context, req *BulkEnqueueRequest) (*BulkEnqueueResponse, error) {
	var result BulkEnqueueResponse
	if err := r.base.Post(ctx, "/api/v1/jobs/bulk", req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// ClaimJobsRequest is the request to claim jobs.
type ClaimJobsRequest struct {
	QueueName        string `json:"queue_name"`
	WorkerID         string `json:"worker_id"`
	Limit            *int   `json:"limit,omitempty"`
	LeaseDurationSec *int   `json:"lease_duration_secs,omitempty"`
}

// ClaimedJob is a job that has been claimed by a worker.
type ClaimedJob struct {
	ID             string         `json:"id"`
	QueueName      string         `json:"queue_name"`
	Payload        map[string]any `json:"payload"`
	RetryCount     int            `json:"retry_count"`
	MaxRetries     int            `json:"max_retries"`
	TimeoutSeconds int            `json:"timeout_seconds"`
	LeaseExpiresAt *time.Time     `json:"lease_expires_at,omitempty"`
}

// ClaimJobsResponse is the response from claiming jobs.
type ClaimJobsResponse struct {
	Jobs []ClaimedJob `json:"jobs"`
}

// Claim claims jobs for a worker.
func (r *JobsResource) Claim(ctx context.Context, req *ClaimJobsRequest) (*ClaimJobsResponse, error) {
	var result ClaimJobsResponse
	if err := r.base.Post(ctx, "/api/v1/jobs/claim", req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// CompleteJobRequest is the request to complete a job.
type CompleteJobRequest struct {
	WorkerID string         `json:"worker_id"`
	Result   map[string]any `json:"result,omitempty"`
}

// Complete marks a job as completed.
func (r *JobsResource) Complete(ctx context.Context, id string, req *CompleteJobRequest) error {
	return r.base.Post(ctx, fmt.Sprintf("/api/v1/jobs/%s/complete", id), req, nil)
}

// FailJobRequest is the request to fail a job.
type FailJobRequest struct {
	WorkerID string `json:"worker_id"`
	Error    string `json:"error"`
}

// Fail marks a job as failed.
func (r *JobsResource) Fail(ctx context.Context, id string, req *FailJobRequest) error {
	return r.base.Post(ctx, fmt.Sprintf("/api/v1/jobs/%s/fail", id), req, nil)
}

// HeartbeatRequest is the request for a job heartbeat.
type HeartbeatRequest struct {
	WorkerID         string `json:"worker_id"`
	LeaseDurationSec *int   `json:"lease_duration_secs,omitempty"`
}

// Heartbeat sends a heartbeat for a job to extend its lease.
func (r *JobsResource) Heartbeat(ctx context.Context, id string, req *HeartbeatRequest) error {
	return r.base.Post(ctx, fmt.Sprintf("/api/v1/jobs/%s/heartbeat", id), req, nil)
}

// RenewLeaseRequest is the request to renew a job's lease.
type RenewLeaseRequest struct {
	WorkerID         string `json:"worker_id"`
	LeaseDurationSec int    `json:"lease_duration_secs,omitempty"`
}

// RenewLeaseResponse is the response from renewing a lease.
type RenewLeaseResponse struct {
	Success        bool       `json:"success"`
	LeaseExpiresAt *time.Time `json:"lease_expires_at,omitempty"`
}

// RenewLease extends the lease on a job.
func (r *JobsResource) RenewLease(ctx context.Context, id string, req *RenewLeaseRequest) (*RenewLeaseResponse, error) {
	var result RenewLeaseResponse
	hbReq := &HeartbeatRequest{
		WorkerID:         req.WorkerID,
		LeaseDurationSec: &req.LeaseDurationSec,
	}
	if err := r.base.Post(ctx, fmt.Sprintf("/api/v1/jobs/%s/heartbeat", id), hbReq, &result); err != nil {
		return nil, err
	}
	result.Success = true
	return &result, nil
}

// UpdateProgressRequest is the request to update job progress.
type UpdateProgressRequest struct {
	Progress float64 `json:"progress"`
	Message  string  `json:"message,omitempty"`
}

// UpdateProgress updates the progress of a job.
func (r *JobsResource) UpdateProgress(ctx context.Context, id string, req *UpdateProgressRequest) error {
	return r.base.Post(ctx, fmt.Sprintf("/api/v1/jobs/%s/progress", id), req, nil)
}

// DLQResource provides access to Dead Letter Queue operations.
type DLQResource struct {
	base *Base
}

// ListDLQParams are parameters for listing DLQ jobs.
type ListDLQParams struct {
	QueueName *string `json:"queue_name,omitempty"`
	Limit     *int    `json:"limit,omitempty"`
	Offset    *int    `json:"offset,omitempty"`
}

// List retrieves dead letter queue jobs.
func (r *DLQResource) List(ctx context.Context, params *ListDLQParams) ([]Job, error) {
	query := url.Values{}
	if params != nil {
		if params.QueueName != nil {
			query.Set("queue_name", *params.QueueName)
		}
		AddPaginationParams(query, params.Limit, params.Offset)
	}

	var result []Job
	if err := r.base.GetWithQuery(ctx, "/api/v1/jobs/dlq", query, &result); err != nil {
		return nil, err
	}
	return result, nil
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

// Retry retries jobs in the dead letter queue.
func (r *DLQResource) Retry(ctx context.Context, req *RetryDLQRequest) (*RetryDLQResponse, error) {
	var result RetryDLQResponse
	if err := r.base.Post(ctx, "/api/v1/jobs/dlq/retry", req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// PurgeDLQRequest is the request to purge DLQ jobs.
type PurgeDLQRequest struct {
	QueueName *string `json:"queue_name,omitempty"`
}

// PurgeDLQResponse is the response from purging DLQ jobs.
type PurgeDLQResponse struct {
	PurgedCount int `json:"purged_count"`
}

// Purge removes jobs from the dead letter queue.
func (r *DLQResource) Purge(ctx context.Context, req *PurgeDLQRequest) (*PurgeDLQResponse, error) {
	var result PurgeDLQResponse
	// Parity with Node/Python: POST /jobs/dlq/purge
	if err := r.base.Post(ctx, "/api/v1/jobs/dlq/purge", req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

