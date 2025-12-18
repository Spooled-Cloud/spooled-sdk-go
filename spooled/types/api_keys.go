package types

import "time"

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

// APIKeySummary is a summary of an API key.
type APIKeySummary struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Queues    []string   `json:"queues,omitempty"`
	RateLimit *int       `json:"rate_limit,omitempty"`
	IsActive  bool       `json:"is_active"`
	CreatedAt time.Time  `json:"created_at"`
	LastUsed  *time.Time `json:"last_used,omitempty"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
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

// UpdateAPIKeyRequest is the request to update an API key.
type UpdateAPIKeyRequest struct {
	Name      *string  `json:"name,omitempty"`
	Queues    []string `json:"queues,omitempty"`
	RateLimit *int     `json:"rate_limit,omitempty"`
	IsActive  *bool    `json:"is_active,omitempty"`
}
