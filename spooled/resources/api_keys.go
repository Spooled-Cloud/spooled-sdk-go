package resources

import (
	"context"
	"fmt"
	"time"

	"github.com/spooled-cloud/spooled-sdk-go/internal/httpx"
)

// APIKeysResource provides access to API key operations.
type APIKeysResource struct {
	base *Base
}

// NewAPIKeysResource creates a new APIKeysResource.
func NewAPIKeysResource(transport *httpx.Transport) *APIKeysResource {
	return &APIKeysResource{base: NewBase(transport)}
}

// APIKey represents an API key.
type APIKey struct {
	ID             string     `json:"id"`
	OrganizationID *string    `json:"organization_id,omitempty"`
	Name           string     `json:"name"`
	KeyPrefix      *string    `json:"key_prefix,omitempty"`
	Queues         []string   `json:"queues,omitempty"`
	RateLimit      *int       `json:"rate_limit,omitempty"`
	IsActive       bool       `json:"is_active"`
	CreatedAt      time.Time  `json:"created_at"`
	LastUsed       *time.Time `json:"last_used,omitempty"`
	ExpiresAt      *time.Time `json:"expires_at,omitempty"`
}

// List retrieves all API keys.
func (r *APIKeysResource) List(ctx context.Context) ([]APIKey, error) {
	var result []APIKey
	if err := r.base.Get(ctx, "/api/v1/api-keys", &result); err != nil {
		return nil, err
	}
	return result, nil
}

// CreateAPIKeyRequest is the request to create an API key.
type CreateAPIKeyRequest struct {
	Name      string     `json:"name"`
	Queues    []string   `json:"queues,omitempty"`
	RateLimit *int       `json:"rate_limit,omitempty"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

// CreateAPIKeyResponse is the response from creating an API key.
type CreateAPIKeyResponse struct {
	ID        string     `json:"id"`
	Key       string     `json:"key"` // Raw key - only shown once!
	Name      string     `json:"name"`
	CreatedAt time.Time  `json:"created_at"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

// Create creates a new API key.
func (r *APIKeysResource) Create(ctx context.Context, req *CreateAPIKeyRequest) (*CreateAPIKeyResponse, error) {
	var result CreateAPIKeyResponse
	if err := r.base.Post(ctx, "/api/v1/api-keys", req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// Get retrieves a specific API key.
func (r *APIKeysResource) Get(ctx context.Context, id string) (*APIKey, error) {
	var result APIKey
	if err := r.base.Get(ctx, fmt.Sprintf("/api/v1/api-keys/%s", id), &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// UpdateAPIKeyRequest is the request to update an API key.
type UpdateAPIKeyRequest struct {
	Name      *string  `json:"name,omitempty"`
	Queues    []string `json:"queues,omitempty"`
	RateLimit *int     `json:"rate_limit,omitempty"`
	IsActive  *bool    `json:"is_active,omitempty"`
}

// Update updates an API key.
func (r *APIKeysResource) Update(ctx context.Context, id string, req *UpdateAPIKeyRequest) (*APIKey, error) {
	var result APIKey
	if err := r.base.Put(ctx, fmt.Sprintf("/api/v1/api-keys/%s", id), req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// Delete deletes an API key.
func (r *APIKeysResource) Delete(ctx context.Context, id string) error {
	return r.base.Delete(ctx, fmt.Sprintf("/api/v1/api-keys/%s", id))
}
