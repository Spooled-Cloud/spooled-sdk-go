package resources

import (
	"context"
	"fmt"
	"net/url"
	"reflect"
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
	ID             string    `json:"id"`
	OrganizationID string    `json:"organization_id"`
	QueueName      string    `json:"queue_name"`
	Status         JobStatus `json:"status"`
	// Payload/Result/Tags are JSON values (object, array, string, bool, …).
	Payload any `json:"payload"`
	Result  any `json:"result,omitempty"`
	// JobType is payload.job_type on list summaries; GET /jobs/{id} has no
	// column and UnmarshalJSON copies it from payload when present.
	JobType           string     `json:"job_type,omitempty"`
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

// UnmarshalJSON maps list/DLQ summaries that send "attempt" onto RetryCount.
// GET /jobs/{id} still sends "retry_count".
func (j *Job) UnmarshalJSON(data []byte) error {
	type jobAlias Job
	aux := struct {
		Attempt *int `json:"attempt"`
		*jobAlias
	}{jobAlias: (*jobAlias)(j)}
	// unmarshalJSON, not json.Unmarshal: keep large integer IDs in Payload exact.
	if err := unmarshalJSON(data, &aux); err != nil {
		return err
	}
	restoreNumbers(reflect.ValueOf(j)) // also for callers that json.Unmarshal a Job directly
	if aux.Attempt != nil && j.RetryCount == 0 {
		j.RetryCount = *aux.Attempt
	}
	if j.JobType == "" {
		if payload, ok := j.Payload.(map[string]any); ok {
			if t, ok := payload["job_type"].(string); ok {
				j.JobType = t
			}
		}
	}
	return nil
}

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
	// CompletionWebhookSecret signs that callback (X-Spooled-Signature).
	// Omitted, the API delivers the completion webhook unsigned.
	CompletionWebhookSecret *string `json:"completion_webhook_secret,omitempty"`
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
//
// Filters combine with AND. The API answers an invalid filter (an unknown
// status, OrderBy or OrderDir, or CreatedAfter not earlier than CreatedBefore)
// with a 400 ValidationError rather than ignoring it.
type ListJobsParams struct {
	QueueName *string `json:"queue_name,omitempty"`
	// Status filters by one status. Statuses filters by several; when both are
	// set the job may have any of them.
	Status *JobStatus `json:"status,omitempty"`
	Tag    *string    `json:"tag,omitempty"` // Filter by a single tag
	Limit  *int       `json:"limit,omitempty"`
	Offset *int       `json:"offset,omitempty"`
	// Statuses keeps jobs in any of these statuses. It is sent as one
	// comma-separated status parameter (status=failed,deadletter).
	Statuses []JobStatus `json:"statuses,omitempty"`
	// Q is a case-insensitive search: a job matches when its ID starts with
	// it, or its queue name or payload job_type contains it. Payloads are not
	// searched.
	Q *string `json:"q,omitempty"`
	// JobType keeps jobs whose payload job_type is exactly this value.
	JobType *string `json:"job_type,omitempty"`
	// CreatedAfter keeps jobs created at or after this instant.
	CreatedAfter *time.Time `json:"created_after,omitempty"`
	// CreatedBefore keeps jobs created before this instant.
	CreatedBefore *time.Time `json:"created_before,omitempty"`
	// OrderBy is the sort column: created_at (default), updated_at,
	// scheduled_at, completed_at, priority, status, queue_name or retry_count.
	OrderBy *string `json:"order_by,omitempty"`
	// OrderDir is "desc" (default) or "asc".
	OrderDir *string `json:"order_dir,omitempty"`
}

// query encodes the parameters for GET /jobs and GET /jobs/dlq.
func (p *ListJobsParams) query() url.Values {
	query := url.Values{}
	if p == nil {
		return query
	}
	if p.QueueName != nil {
		query.Set("queue_name", *p.QueueName)
	}
	statuses := make([]string, 0, len(p.Statuses)+1)
	if p.Status != nil {
		statuses = append(statuses, string(*p.Status))
	}
	for _, status := range p.Statuses {
		statuses = append(statuses, string(status))
	}
	if len(statuses) > 0 {
		query.Set("status", strings.Join(statuses, ","))
	}
	if p.Tag != nil {
		query.Set("tag", *p.Tag)
	}
	if p.Q != nil {
		query.Set("q", *p.Q)
	}
	if p.JobType != nil {
		query.Set("job_type", *p.JobType)
	}
	setTimeParam(query, "created_after", p.CreatedAfter)
	setTimeParam(query, "created_before", p.CreatedBefore)
	if p.OrderBy != nil {
		query.Set("order_by", *p.OrderBy)
	}
	if p.OrderDir != nil {
		query.Set("order_dir", *p.OrderDir)
	}
	AddPaginationParams(query, p.Limit, p.Offset)
	return query
}

// setTimeParam sends t as an RFC 3339 instant in UTC, the form the API parses.
func setTimeParam(query url.Values, key string, t *time.Time) {
	if t != nil {
		query.Set(key, t.UTC().Format(time.RFC3339Nano))
	}
}

// JobsPage is one page of a job list.
type JobsPage struct {
	Jobs []Job
	// HasMore is the API's X-Has-More header: true when this page is full and
	// at least one more job matches. Ask for the next page with Offset
	// advanced by len(Jobs).
	HasMore bool
}

// List retrieves a list of jobs. ListPage also reports whether more follow.
func (r *JobsResource) List(ctx context.Context, params *ListJobsParams) ([]Job, error) {
	page, err := r.ListPage(ctx, params)
	if err != nil {
		return nil, err
	}
	return page.Jobs, nil
}

// ListPage retrieves one page of jobs together with the X-Has-More hint.
func (r *JobsResource) ListPage(ctx context.Context, params *ListJobsParams) (*JobsPage, error) {
	var page JobsPage
	hasMore, err := r.base.getPage(ctx, "/api/v1/jobs", params.query(), &page.Jobs)
	if err != nil {
		return nil, err
	}
	page.HasMore = hasMore
	return &page, nil
}

// JobHistoryEvent is one recorded lifecycle event of a job.
type JobHistoryEvent struct {
	ID    string `json:"id"`
	JobID string `json:"job_id"`
	// EventType names the event, such as "created", "claimed", "completed" or
	// "failed". Compare it as a string: the API can add event types.
	EventType string `json:"event_type"`
	// WorkerID is the worker involved, when there was one.
	WorkerID *string `json:"worker_id,omitempty"`
	// Details is the event's JSON details (any JSON value).
	Details   any       `json:"details,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// JobHistoryParams are parameters for reading a job's history.
type JobHistoryParams struct {
	// Order is "asc" (default, oldest event first) or "desc" (newest first).
	Order *string `json:"order_dir,omitempty"`
	// Limit is the page size, 1-500 (API default 100).
	Limit  *int `json:"limit,omitempty"`
	Offset *int `json:"offset,omitempty"`
}

// JobHistoryPage is one page of a job's history.
type JobHistoryPage struct {
	Events []JobHistoryEvent
	// HasMore is the API's X-Has-More header: true when more events follow.
	HasMore bool
}

// History returns a job's recorded lifecycle events
// (GET /jobs/{id}/history), oldest first unless params.Order is "desc".
// Another organization's job, or one outside an API key's queue scope, is a
// NotFoundError.
func (r *JobsResource) History(ctx context.Context, id string, params *JobHistoryParams) (*JobHistoryPage, error) {
	query := url.Values{}
	if params != nil {
		if params.Order != nil {
			query.Set("order_dir", *params.Order)
		}
		AddPaginationParams(query, params.Limit, params.Offset)
	}
	var page JobHistoryPage
	hasMore, err := r.base.getPage(ctx, fmt.Sprintf("/api/v1/jobs/%s/history", id), query, &page.Events)
	if err != nil {
		return nil, err
	}
	if page.Events == nil {
		page.Events = []JobHistoryEvent{}
	}
	page.HasMore = hasMore
	return &page, nil
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
// GET /jobs/status sends retry_count, not attempt, and has no max_retries.
type BatchJobStatus struct {
	ID          string     `json:"id"`
	Status      JobStatus  `json:"status"`
	QueueName   string     `json:"queue_name"`
	RetryCount  int        `json:"retry_count"`
	CreatedAt   time.Time  `json:"created_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
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
//
// Payload is `any` for the same reason as CreateJobRequest.Payload: the API
// field is `serde_json::Value`, not an object.
type BulkJobItem struct {
	Payload        any        `json:"payload"`
	Priority       *int       `json:"priority,omitempty"`
	IdempotencyKey *string    `json:"idempotency_key,omitempty"`
	ScheduledAt    *time.Time `json:"scheduled_at,omitempty"`
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
	ID             string     `json:"id"`
	QueueName      string     `json:"queue_name"`
	Payload        any        `json:"payload"`
	RetryCount     int        `json:"retry_count"`
	MaxRetries     int        `json:"max_retries"`
	TimeoutSeconds int        `json:"timeout_seconds"`
	LeaseExpiresAt *time.Time `json:"lease_expires_at,omitempty"`
	// LeaseID is the lease fencing token issued by the server for this claim.
	// Echo it back on Complete, Fail, Heartbeat and RenewLease: the API
	// requires it there (422 VALIDATION_ERROR without it) and applies the call
	// only to the lease this worker actually holds.
	LeaseID *string `json:"lease_id,omitempty"`
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
	WorkerID string `json:"worker_id"`
	Result   any    `json:"result,omitempty"`
	// LeaseID is the lease fencing token from the claimed job
	// (ClaimedJob.LeaseID). It is required: the API answers a complete without
	// it with 422 VALIDATION_ERROR ("Field `lease_id` is required"), and one
	// that does not match the job's current lease with 409. It is a pointer
	// only for compatibility; always set it.
	LeaseID *string `json:"lease_id,omitempty"`
}

// Complete marks a job as completed. The call is retried on transient failures
// (see Base.PostAck): it is lease-fenced, so a repeat cannot complete twice.
func (r *JobsResource) Complete(ctx context.Context, id string, req *CompleteJobRequest) error {
	return r.base.PostAck(ctx, fmt.Sprintf("/api/v1/jobs/%s/complete", id), req, nil)
}

// FailJobRequest is the request to fail a job.
type FailJobRequest struct {
	WorkerID string `json:"worker_id"`
	Error    string `json:"error"`
	// LeaseID is the lease fencing token from the claimed job
	// (ClaimedJob.LeaseID). It is required: the API answers a fail without it
	// with 422 VALIDATION_ERROR ("Field `lease_id` is required"), and one that
	// does not match the job's current lease with 409. It is a pointer only for
	// compatibility; always set it.
	LeaseID *string `json:"lease_id,omitempty"`
}

// FailJobResponse is returned after the backend accepts a job failure.
type FailJobResponse struct {
	Success bool `json:"success"`
	// WillRetry and NextRetryDelaySec are filled only when the API sends them.
	// POST /jobs/{id}/fail on Spooled Cloud currently answers {"success":true}
	// alone, so both are nil there: decide from the claimed job instead (it is
	// retried while RetryCount < MaxRetries), or fail over gRPC, whose
	// FailWithResponse returns the server's decision.
	WillRetry         *bool `json:"will_retry,omitempty"`
	NextRetryDelaySec *int  `json:"next_retry_delay_secs,omitempty"`
}

// MaxFailureMessageLength is the longest failure message POST /jobs/{id}/fail
// accepts, counted in characters.
const MaxFailureMessageLength = 2048

// DefaultFailureMessage replaces an empty failure message.
const DefaultFailureMessage = "Job failed"

// CoerceFailureMessage makes a failure message acceptable to the API, which
// rejects an empty one or one over MaxFailureMessageLength characters with a 400.
// A rejected /fail leaves the job "processing" until its lease expires, so the
// real error is lost and the retry is delayed by the whole lease.
//
// An empty (or blank) message becomes "Job failed"; a longer one is cut to 2048
// characters on a character boundary, never in the middle of a UTF-8 sequence.
func CoerceFailureMessage(msg string) string {
	if strings.TrimSpace(msg) == "" {
		return DefaultFailureMessage
	}
	if len(msg) <= MaxFailureMessageLength {
		// Fewer bytes than the limit means fewer characters too.
		return msg
	}
	count := 0
	for i := range msg {
		if count == MaxFailureMessageLength {
			return msg[:i]
		}
		count++
	}
	return msg
}

// Fail marks a job as failed.
func (r *JobsResource) Fail(ctx context.Context, id string, req *FailJobRequest) error {
	_, err := r.FailWithResponse(ctx, id, req)
	return err
}

// FailWithResponse marks a job as failed and returns the backend's retry
// disposition when available.
//
// The error text is passed through CoerceFailureMessage so the API cannot reject
// it, and the call is retried on transient failures like Complete.
func (r *JobsResource) FailWithResponse(ctx context.Context, id string, req *FailJobRequest) (*FailJobResponse, error) {
	body := FailJobRequest{}
	if req != nil {
		body = *req // do not mutate the caller's request
	}
	body.Error = CoerceFailureMessage(body.Error)

	var result FailJobResponse
	if err := r.base.PostAck(ctx, fmt.Sprintf("/api/v1/jobs/%s/fail", id), &body, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// HeartbeatRequest is the request for a job heartbeat.
type HeartbeatRequest struct {
	WorkerID         string `json:"worker_id"`
	LeaseDurationSec *int   `json:"lease_duration_secs,omitempty"`
	// LeaseID is the lease fencing token from the claimed job
	// (ClaimedJob.LeaseID). It is required: the API answers a heartbeat without
	// it with 422 VALIDATION_ERROR, and one that does not match the job's
	// current lease with 409. It is a pointer only for compatibility; always
	// set it.
	LeaseID *string `json:"lease_id,omitempty"`
}

// Heartbeat sends a heartbeat for a job to extend its lease. It is retried on
// transient failures (see Base.PostAck).
func (r *JobsResource) Heartbeat(ctx context.Context, id string, req *HeartbeatRequest) error {
	return r.base.PostAck(ctx, fmt.Sprintf("/api/v1/jobs/%s/heartbeat", id), req, nil)
}

// RenewLeaseRequest is the request to renew a job's lease.
type RenewLeaseRequest struct {
	WorkerID         string `json:"worker_id"`
	LeaseDurationSec int    `json:"lease_duration_secs,omitempty"`
	// LeaseID is the lease fencing token from the claimed job
	// (ClaimedJob.LeaseID). It is required: RenewLease sends a job heartbeat,
	// which the API answers with 422 VALIDATION_ERROR without it. It is a
	// pointer only for compatibility; always set it.
	LeaseID *string `json:"lease_id,omitempty"`
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
		LeaseID:          req.LeaseID,
	}
	if err := r.base.PostAck(ctx, fmt.Sprintf("/api/v1/jobs/%s/heartbeat", id), hbReq, &result); err != nil {
		return nil, err
	}
	result.Success = true
	return &result, nil
}

// UpdateProgressRequest is the request to update job progress.
//
// Deprecated: the Spooled API has no job progress route.
type UpdateProgressRequest struct {
	Progress float64 `json:"progress"`
	Message  string  `json:"message,omitempty"`
}

// UpdateProgress always returns an UnsupportedError and makes no request.
//
// Deprecated: the Spooled API has no job progress route
// (POST /jobs/{id}/progress was answered with 404 every time). Put progress in
// the job result, or log it from the handler with JobContext.Log.
func (r *JobsResource) UpdateProgress(_ context.Context, _ string, _ *UpdateProgressRequest) error {
	return &UnsupportedError{
		Operation: "Jobs.UpdateProgress",
		Reason:    "the API has no job progress route; return progress in the job result or log it with JobContext.Log",
	}
}

// DLQResource provides access to Dead Letter Queue operations.
type DLQResource struct {
	base *Base
}

// ListDLQParams are parameters for listing DLQ jobs. The filters work as on
// ListJobsParams.
type ListDLQParams struct {
	QueueName *string `json:"queue_name,omitempty"`
	Limit     *int    `json:"limit,omitempty"`
	Offset    *int    `json:"offset,omitempty"`
	// Tag keeps jobs with this tag.
	Tag *string `json:"tag,omitempty"`
	// Q is the same search as ListJobsParams.Q.
	Q *string `json:"q,omitempty"`
	// JobType keeps jobs whose payload job_type is exactly this value.
	JobType *string `json:"job_type,omitempty"`
	// CreatedAfter keeps jobs created at or after this instant.
	CreatedAfter *time.Time `json:"created_after,omitempty"`
	// CreatedBefore keeps jobs created before this instant.
	CreatedBefore *time.Time `json:"created_before,omitempty"`
}

// List retrieves dead letter queue jobs. ListPage also reports whether more
// follow.
func (r *DLQResource) List(ctx context.Context, params *ListDLQParams) ([]Job, error) {
	page, err := r.ListPage(ctx, params)
	if err != nil {
		return nil, err
	}
	return page.Jobs, nil
}

// ListPage retrieves one page of dead-lettered jobs together with the
// X-Has-More hint.
func (r *DLQResource) ListPage(ctx context.Context, params *ListDLQParams) (*JobsPage, error) {
	var query url.Values
	if params != nil {
		query = (&ListJobsParams{
			QueueName:     params.QueueName,
			Limit:         params.Limit,
			Offset:        params.Offset,
			Tag:           params.Tag,
			Q:             params.Q,
			JobType:       params.JobType,
			CreatedAfter:  params.CreatedAfter,
			CreatedBefore: params.CreatedBefore,
		}).query()
	}

	var page JobsPage
	hasMore, err := r.base.getPage(ctx, "/api/v1/jobs/dlq", query, &page.Jobs)
	if err != nil {
		return nil, err
	}
	page.HasMore = hasMore
	return &page, nil
}

// RetryDLQRequest is the request to retry DLQ jobs.
type RetryDLQRequest struct {
	QueueName *string  `json:"queue_name,omitempty"`
	JobIDs    []string `json:"job_ids,omitempty"`
	// Limit caps how many jobs one call retries. The API default and max are
	// 100; without this field every call uses that default.
	Limit *int `json:"limit,omitempty"`
}

// RetryDLQResponse is the response from retrying DLQ jobs.
type RetryDLQResponse struct {
	RetriedCount int      `json:"retried_count"`
	RetriedJobs  []string `json:"retried_jobs,omitempty"`
	// SkippedJobs stayed dead-lettered: reviving their workflow would pass
	// the plan's max_workflows. A partial retry is still HTTP 200, so these
	// ids are the only sign those jobs were not retried.
	SkippedJobs []string `json:"skipped_jobs,omitempty"`
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
	// OlderThan purges only jobs created before this time.
	OlderThan *time.Time `json:"older_than,omitempty"`
	// Limit caps how many jobs one call purges (API default and max: 10000).
	Limit *int `json:"limit,omitempty"`
	// Confirm must be true: the API rejects a purge without it. Purge sets it,
	// since calling Purge is the confirmation.
	Confirm bool `json:"confirm"`
}

// PurgeDLQResponse is the response from purging DLQ jobs.
type PurgeDLQResponse struct {
	PurgedCount int `json:"purged_count"`
}

// Purge removes jobs from the dead letter queue.
func (r *DLQResource) Purge(ctx context.Context, req *PurgeDLQRequest) (*PurgeDLQResponse, error) {
	var result PurgeDLQResponse
	// The API requires "confirm": true; without it every purge failed with a
	// validation error. Copy so the caller's struct is not mutated.
	body := PurgeDLQRequest{Confirm: true}
	if req != nil {
		body = *req
		body.Confirm = true
	}
	// Parity with Node/Python: POST /jobs/dlq/purge
	if err := r.base.Post(ctx, "/api/v1/jobs/dlq/purge", &body, &result); err != nil {
		return nil, err
	}
	return &result, nil
}
