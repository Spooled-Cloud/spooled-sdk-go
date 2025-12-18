package resources

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/spooled-cloud/spooled-sdk-go/internal/httpx"
)

// WorkflowsResource provides access to workflow operations.
type WorkflowsResource struct {
	base *Base
	jobs *WorkflowJobsResource
}

// NewWorkflowsResource creates a new WorkflowsResource.
func NewWorkflowsResource(transport *httpx.Transport) *WorkflowsResource {
	base := NewBase(transport)
	return &WorkflowsResource{
		base: base,
		jobs: &WorkflowJobsResource{base: base},
	}
}

// Jobs returns the workflow jobs sub-resource.
func (r *WorkflowsResource) Jobs() *WorkflowJobsResource {
	return r.jobs
}

// WorkflowStatus represents the status of a workflow.
type WorkflowStatus string

const (
	WorkflowStatusPending   WorkflowStatus = "pending"
	WorkflowStatusRunning   WorkflowStatus = "running"
	WorkflowStatusCompleted WorkflowStatus = "completed"
	WorkflowStatusFailed    WorkflowStatus = "failed"
	WorkflowStatusCancelled WorkflowStatus = "cancelled"
)

// Workflow represents a workflow.
type Workflow struct {
	ID             string         `json:"id"`
	OrganizationID string         `json:"organization_id"`
	Name           string         `json:"name"`
	Description    *string        `json:"description,omitempty"`
	Status         WorkflowStatus `json:"status"`
	TotalJobs      int            `json:"total_jobs"`
	CompletedJobs  int            `json:"completed_jobs"`
	FailedJobs     int            `json:"failed_jobs"`
	CreatedAt      time.Time      `json:"created_at"`
	StartedAt      *time.Time     `json:"started_at,omitempty"`
	CompletedAt    *time.Time     `json:"completed_at,omitempty"`
	Metadata       map[string]any `json:"metadata,omitempty"`
}

// ListWorkflowsParams are parameters for listing workflows.
type ListWorkflowsParams struct {
	Status *WorkflowStatus `json:"status,omitempty"`
	Limit  *int            `json:"limit,omitempty"`
	Offset *int            `json:"offset,omitempty"`
}

// List retrieves all workflows.
func (r *WorkflowsResource) List(ctx context.Context, params *ListWorkflowsParams) ([]Workflow, error) {
	query := url.Values{}
	if params != nil {
		if params.Status != nil {
			query.Set("status", string(*params.Status))
		}
		AddPaginationParams(query, params.Limit, params.Offset)
	}

	var result []Workflow
	if err := r.base.GetWithQuery(ctx, "/api/v1/workflows", query, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// DependencyMode specifies how job dependencies are evaluated.
type DependencyMode string

const (
	DependencyModeAll DependencyMode = "all"
	DependencyModeAny DependencyMode = "any"
)

// WorkflowJobDefinition defines a job within a workflow.
type WorkflowJobDefinition struct {
	Key            string         `json:"key"`
	QueueName      string         `json:"queue_name"`
	Payload        map[string]any `json:"payload"`
	DependsOn      []string       `json:"depends_on,omitempty"`
	DependencyMode *DependencyMode `json:"dependency_mode,omitempty"`
	Priority       *int           `json:"priority,omitempty"`
	MaxRetries     *int           `json:"max_retries,omitempty"`
	TimeoutSeconds *int           `json:"timeout_seconds,omitempty"`
}

// CreateWorkflowRequest is the request to create a workflow.
type CreateWorkflowRequest struct {
	Name        string                  `json:"name"`
	Description *string                 `json:"description,omitempty"`
	Jobs        []WorkflowJobDefinition `json:"jobs"`
	Metadata    map[string]any          `json:"metadata,omitempty"`
}

// WorkflowJobMapping maps a workflow job key to its job ID.
type WorkflowJobMapping struct {
	Key   string `json:"key"`
	JobID string `json:"job_id"`
}

// CreateWorkflowResponse is the response from creating a workflow.
type CreateWorkflowResponse struct {
	WorkflowID string               `json:"workflow_id"`
	JobIDs     []WorkflowJobMapping `json:"job_ids"`
}

// Create creates a new workflow.
func (r *WorkflowsResource) Create(ctx context.Context, req *CreateWorkflowRequest) (*CreateWorkflowResponse, error) {
	var result CreateWorkflowResponse
	if err := r.base.Post(ctx, "/api/v1/workflows", req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// Get retrieves a specific workflow.
func (r *WorkflowsResource) Get(ctx context.Context, id string) (*Workflow, error) {
	var result Workflow
	if err := r.base.Get(ctx, fmt.Sprintf("/api/v1/workflows/%s", id), &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// Cancel cancels a workflow.
func (r *WorkflowsResource) Cancel(ctx context.Context, id string) error {
	return r.base.Post(ctx, fmt.Sprintf("/api/v1/workflows/%s/cancel", id), nil, nil)
}

// Retry retries a failed workflow.
func (r *WorkflowsResource) Retry(ctx context.Context, id string) (*Workflow, error) {
	var result Workflow
	if err := r.base.Post(ctx, fmt.Sprintf("/api/v1/workflows/%s/retry", id), nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// WorkflowJobsResource provides access to workflow job operations.
type WorkflowJobsResource struct {
	base *Base
}

// WorkflowJob represents a job within a workflow with dependencies info.
type WorkflowJob struct {
	ID             string     `json:"id"`
	Key            string     `json:"key"`
	QueueName      string     `json:"queue_name"`
	Status         JobStatus  `json:"status"`
	DependsOn      []string   `json:"depends_on,omitempty"`
	DependencyMode *string    `json:"dependency_mode,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	StartedAt      *time.Time `json:"started_at,omitempty"`
	CompletedAt    *time.Time `json:"completed_at,omitempty"`
}

// ListJobs retrieves all jobs in a workflow.
func (r *WorkflowJobsResource) ListJobs(ctx context.Context, workflowID string) ([]WorkflowJob, error) {
	var result []WorkflowJob
	if err := r.base.Get(ctx, fmt.Sprintf("/api/v1/workflows/%s/jobs", workflowID), &result); err != nil {
		return nil, err
	}
	return result, nil
}

// GetJob retrieves a specific job in a workflow.
func (r *WorkflowJobsResource) GetJob(ctx context.Context, workflowID, jobID string) (*WorkflowJob, error) {
	var result WorkflowJob
	if err := r.base.Get(ctx, fmt.Sprintf("/api/v1/workflows/%s/jobs/%s", workflowID, jobID), &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// WorkflowJobStatus represents the status of jobs in a workflow.
type WorkflowJobStatusResponse struct {
	Jobs []WorkflowJobStatus `json:"jobs"`
}

// WorkflowJobStatus represents a job's status in a workflow.
type WorkflowJobStatus struct {
	Key       string    `json:"key"`
	JobID     string    `json:"job_id"`
	Status    JobStatus `json:"status"`
	DependsOn []string  `json:"depends_on,omitempty"`
}

// GetJobsStatus retrieves the status of all jobs in a workflow.
func (r *WorkflowJobsResource) GetJobsStatus(ctx context.Context, workflowID string) (*WorkflowJobStatusResponse, error) {
	var result WorkflowJobStatusResponse
	if err := r.base.Get(ctx, fmt.Sprintf("/api/v1/workflows/%s/jobs/status", workflowID), &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// Job dependencies (can be used standalone)

// JobWithDependencies represents a job with its dependencies.
type JobWithDependencies struct {
	Job          Job      `json:"job"`
	Dependencies []string `json:"dependencies"`
	Dependents   []string `json:"dependents"`
}

// GetJobDependencies retrieves dependencies for a job.
func (r *WorkflowsResource) GetJobDependencies(ctx context.Context, jobID string) (*JobWithDependencies, error) {
	var result JobWithDependencies
	if err := r.base.Get(ctx, fmt.Sprintf("/api/v1/jobs/%s/dependencies", jobID), &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// AddDependenciesRequest is the request to add job dependencies.
type AddDependenciesRequest struct {
	DependsOn      []string        `json:"depends_on"`
	DependencyMode *DependencyMode `json:"dependency_mode,omitempty"`
}

// AddDependenciesResponse is the response from adding dependencies.
type AddDependenciesResponse struct {
	Success      bool     `json:"success"`
	Dependencies []string `json:"dependencies"`
}

// AddJobDependencies adds dependencies to a job.
func (r *WorkflowsResource) AddJobDependencies(ctx context.Context, jobID string, req *AddDependenciesRequest) (*AddDependenciesResponse, error) {
	var result AddDependenciesResponse
	if err := r.base.Post(ctx, fmt.Sprintf("/api/v1/jobs/%s/dependencies", jobID), req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}


