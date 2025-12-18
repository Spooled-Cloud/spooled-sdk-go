package types

import "time"

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
	Metadata       *JsonObject    `json:"metadata,omitempty"`
}

// WorkflowResponse is the response for workflow operations.
type WorkflowResponse struct {
	ID              string         `json:"id"`
	Name            string         `json:"name"`
	Status          WorkflowStatus `json:"status"`
	TotalJobs       int            `json:"total_jobs"`
	CompletedJobs   int            `json:"completed_jobs"`
	FailedJobs      int            `json:"failed_jobs"`
	ProgressPercent float64        `json:"progress_percent"`
	CreatedAt       time.Time      `json:"created_at"`
	CompletedAt     *time.Time     `json:"completed_at,omitempty"`
}

// CreateWorkflowRequest is the request to create a workflow.
type CreateWorkflowRequest struct {
	Name        string                  `json:"name"`
	Description *string                 `json:"description,omitempty"`
	Jobs        []WorkflowJobDefinition `json:"jobs"`
	Metadata    *JsonObject             `json:"metadata,omitempty"`
}

// DependencyMode specifies how job dependencies are evaluated.
type DependencyMode string

const (
	DependencyModeAll DependencyMode = "all"
	DependencyModeAny DependencyMode = "any"
)

// WorkflowJobDefinition defines a job within a workflow.
type WorkflowJobDefinition struct {
	Key            string          `json:"key"`
	QueueName      string          `json:"queue_name"`
	Payload        JsonObject      `json:"payload"`
	DependsOn      []string        `json:"depends_on,omitempty"`
	DependencyMode *DependencyMode `json:"dependency_mode,omitempty"`
	Priority       *int            `json:"priority,omitempty"`
	MaxRetries     *int            `json:"max_retries,omitempty"`
	TimeoutSeconds *int            `json:"timeout_seconds,omitempty"`
}

// CreateWorkflowResponse is the response from creating a workflow.
type CreateWorkflowResponse struct {
	WorkflowID string               `json:"workflow_id"`
	JobIDs     []WorkflowJobMapping `json:"job_ids"`
}

// WorkflowJobMapping maps a workflow job key to its job ID.
type WorkflowJobMapping struct {
	Key   string `json:"key"`
	JobID string `json:"job_id"`
}

// ListWorkflowsParams are parameters for listing workflows.
type ListWorkflowsParams struct {
	Status *WorkflowStatus `json:"status,omitempty"`
	Limit  *int            `json:"limit,omitempty"`
	Offset *int            `json:"offset,omitempty"`
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

// WorkflowJobStatus represents the status of jobs in a workflow.
type WorkflowJobStatus struct {
	Key       string    `json:"key"`
	JobID     string    `json:"job_id"`
	Status    JobStatus `json:"status"`
	DependsOn []string  `json:"depends_on,omitempty"`
}

// JobWithDependencies represents a job with its dependencies.
type JobWithDependencies struct {
	Job          Job      `json:"job"`
	Dependencies []string `json:"dependencies"`
	Dependents   []string `json:"dependents"`
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
