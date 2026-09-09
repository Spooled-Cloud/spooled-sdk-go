package resources

import (
	"context"
	"net/http"

	"github.com/spooled-cloud/spooled-sdk-go/internal/httpx"
)

// MetricsResource provides access to metrics operations.
type MetricsResource struct {
	base *Base
}

// NewMetricsResource creates a new MetricsResource.
func NewMetricsResource(transport *httpx.Transport) *MetricsResource {
	return &MetricsResource{base: NewBase(transport)}
}

// Metrics represents system metrics.
type Metrics struct {
	Jobs    JobMetrics    `json:"jobs"`
	Workers WorkerMetrics `json:"workers"`
	Queues  QueueMetrics  `json:"queues"`
	System  SystemMetrics `json:"system"`
}

// JobMetrics contains job-related metrics.
type JobMetrics struct {
	TotalCreated    int64   `json:"total_created"`
	TotalCompleted  int64   `json:"total_completed"`
	TotalFailed     int64   `json:"total_failed"`
	AvgWaitTimeMs   float64 `json:"avg_wait_time_ms"`
	AvgProcessingMs float64 `json:"avg_processing_ms"`
}

// WorkerMetrics contains worker-related metrics.
type WorkerMetrics struct {
	TotalRegistered int `json:"total_registered"`
	TotalActive     int `json:"total_active"`
	TotalIdle       int `json:"total_idle"`
}

// QueueMetrics contains queue-related metrics.
type QueueMetrics struct {
	TotalQueues     int `json:"total_queues"`
	TotalPaused     int `json:"total_paused"`
	TotalPending    int `json:"total_pending"`
	TotalProcessing int `json:"total_processing"`
}

// SystemMetrics contains system-level metrics.
type SystemMetrics struct {
	UptimeSeconds  int64   `json:"uptime_seconds"`
	RequestsPerSec float64 `json:"requests_per_sec"`
	ErrorRate      float64 `json:"error_rate"`
	AvgLatencyMs   float64 `json:"avg_latency_ms"`
}

// Get retrieves Prometheus text from GET /metrics (not under /api/v1).
func (r *MetricsResource) Get(ctx context.Context) (string, error) {
	resp, err := r.base.transport.Do(ctx, &httpx.Request{
		Method: http.MethodGet,
		Path:   "/metrics",
	})
	if err != nil {
		return "", err
	}
	if resp == nil {
		return "", nil
	}
	return string(resp.Body), nil
}

// Prometheus is GET /metrics. There is no /api/v1/metrics/prometheus route.
func (r *MetricsResource) Prometheus(ctx context.Context) (string, error) {
	return r.Get(ctx)
}
