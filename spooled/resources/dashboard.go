package resources

import (
	"context"

	"github.com/spooled-cloud/spooled-sdk-go/internal/httpx"
)

// DashboardResource provides access to dashboard operations.
type DashboardResource struct {
	base *Base
}

// NewDashboardResource creates a new DashboardResource.
func NewDashboardResource(transport *httpx.Transport) *DashboardResource {
	return &DashboardResource{base: NewBase(transport)}
}

// DashboardData represents aggregated dashboard data.
type DashboardData struct {
	System         SystemInfo        `json:"system"`
	Jobs           JobSummaryStats   `json:"jobs"`
	Queues         []QueueSummary    `json:"queues"`
	Workers        WorkerSummaryInfo `json:"workers"`
	RecentActivity RecentActivity    `json:"recent_activity"`
}

// SystemInfo contains system information.
type SystemInfo struct {
	Version        string `json:"version"`
	UptimeSeconds  int64  `json:"uptime_seconds"`
	StartedAt      string `json:"started_at"`
	DatabaseStatus string `json:"database_status"`
	CacheStatus    string `json:"cache_status"`
	Environment    string `json:"environment"`
}

// JobSummaryStats contains job statistics for the dashboard.
type JobSummaryStats struct {
	Total               int      `json:"total"`
	Pending             int      `json:"pending"`
	Processing          int      `json:"processing"`
	Completed24h        int      `json:"completed_24h"`
	Failed24h           int      `json:"failed_24h"`
	Deadletter          int      `json:"deadletter"`
	AvgWaitTimeMs       *float64 `json:"avg_wait_time_ms,omitempty"`
	AvgProcessingTimeMs *float64 `json:"avg_processing_time_ms,omitempty"`
}

// QueueSummary is a queue summary for the dashboard.
type QueueSummary struct {
	Name       string `json:"name"`
	Pending    int    `json:"pending"`
	Processing int    `json:"processing"`
	Paused     bool   `json:"paused"`
}

// WorkerSummaryInfo contains worker summary information.
type WorkerSummaryInfo struct {
	Total     int `json:"total"`
	Healthy   int `json:"healthy"`
	Unhealthy int `json:"unhealthy"`
}

// RecentActivity contains recent activity information.
type RecentActivity struct {
	JobsCreated1h   int `json:"jobs_created_1h"`
	JobsCompleted1h int `json:"jobs_completed_1h"`
	JobsFailed1h    int `json:"jobs_failed_1h"`
}

// Get retrieves the dashboard data.
func (r *DashboardResource) Get(ctx context.Context) (*DashboardData, error) {
	var result DashboardData
	if err := r.base.Get(ctx, "/api/v1/dashboard", &result); err != nil {
		return nil, err
	}
	return &result, nil
}
