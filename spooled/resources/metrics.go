package resources

import (
	"context"

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
	Jobs     JobMetrics     `json:"jobs"`
	Workers  WorkerMetrics  `json:"workers"`
	Queues   QueueMetrics   `json:"queues"`
	System   SystemMetrics  `json:"system"`
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
	TotalQueues    int `json:"total_queues"`
	TotalPaused    int `json:"total_paused"`
	TotalPending   int `json:"total_pending"`
	TotalProcessing int `json:"total_processing"`
}

// SystemMetrics contains system-level metrics.
type SystemMetrics struct {
	UptimeSeconds   int64  `json:"uptime_seconds"`
	RequestsPerSec  float64 `json:"requests_per_sec"`
	ErrorRate       float64 `json:"error_rate"`
	AvgLatencyMs    float64 `json:"avg_latency_ms"`
}

// Get retrieves system metrics.
func (r *MetricsResource) Get(ctx context.Context) (*Metrics, error) {
	var result Metrics
	if err := r.base.Get(ctx, "/api/v1/metrics", &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// Prometheus retrieves metrics in Prometheus format.
func (r *MetricsResource) Prometheus(ctx context.Context) (string, error) {
	var result string
	if err := r.base.Get(ctx, "/api/v1/metrics/prometheus", &result); err != nil {
		return "", err
	}
	return result, nil
}


