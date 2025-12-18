package resources

import (
	"context"

	"github.com/spooled-cloud/spooled-sdk-go/internal/httpx"
)

// HealthResource provides access to health check operations.
type HealthResource struct {
	base *Base
}

// NewHealthResource creates a new HealthResource.
func NewHealthResource(transport *httpx.Transport) *HealthResource {
	return &HealthResource{base: NewBase(transport)}
}

// HealthResponse represents the health check response.
type HealthResponse struct {
	Status    string `json:"status"`
	Database  bool   `json:"database"`
	Cache     bool   `json:"cache"`
	Timestamp string `json:"timestamp,omitempty"`
}

// Get performs a full health check.
func (r *HealthResource) Get(ctx context.Context) (*HealthResponse, error) {
	var result HealthResponse
	if err := r.base.Get(ctx, "/health", &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// Liveness checks if the service is alive.
func (r *HealthResource) Liveness(ctx context.Context) (*HealthResponse, error) {
	var result HealthResponse
	if err := r.base.Get(ctx, "/health/live", &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// Readiness checks if the service is ready to serve requests.
func (r *HealthResource) Readiness(ctx context.Context) (*HealthResponse, error) {
	var result HealthResponse
	if err := r.base.Get(ctx, "/health/ready", &result); err != nil {
		return nil, err
	}
	return &result, nil
}


