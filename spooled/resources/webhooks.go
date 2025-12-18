package resources

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/spooled-cloud/spooled-sdk-go/internal/httpx"
)

// WebhooksResource provides access to outgoing webhook operations.
type WebhooksResource struct {
	base *Base
}

// NewWebhooksResource creates a new WebhooksResource.
func NewWebhooksResource(transport *httpx.Transport) *WebhooksResource {
	return &WebhooksResource{base: NewBase(transport)}
}

// WebhookEvent represents a webhook event type.
type WebhookEvent string

const (
	WebhookEventJobCreated         WebhookEvent = "job.created"
	WebhookEventJobStarted         WebhookEvent = "job.started"
	WebhookEventJobCompleted       WebhookEvent = "job.completed"
	WebhookEventJobFailed          WebhookEvent = "job.failed"
	WebhookEventJobCancelled       WebhookEvent = "job.cancelled"
	WebhookEventQueuePaused        WebhookEvent = "queue.paused"
	WebhookEventQueueResumed       WebhookEvent = "queue.resumed"
	WebhookEventWorkerRegistered   WebhookEvent = "worker.registered"
	WebhookEventWorkerDeregistered WebhookEvent = "worker.deregistered"
	WebhookEventScheduleTriggered  WebhookEvent = "schedule.triggered"
)

// OutgoingWebhook represents an outgoing webhook configuration.
type OutgoingWebhook struct {
	ID              string         `json:"id"`
	OrganizationID  string         `json:"organization_id"`
	Name            string         `json:"name"`
	URL             string         `json:"url"`
	Events          []WebhookEvent `json:"events"`
	Enabled         bool           `json:"enabled"`
	FailureCount    int            `json:"failure_count"`
	LastTriggeredAt *time.Time     `json:"last_triggered_at,omitempty"`
	LastStatus      *string        `json:"last_status,omitempty"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
}

// List retrieves all outgoing webhooks.
func (r *WebhooksResource) List(ctx context.Context) ([]OutgoingWebhook, error) {
	var result []OutgoingWebhook
	// Parity with Node/Python: /outgoing-webhooks
	if err := r.base.Get(ctx, "/api/v1/outgoing-webhooks", &result); err != nil {
		return nil, err
	}
	return result, nil
}

// CreateOutgoingWebhookRequest is the request to create an outgoing webhook.
type CreateOutgoingWebhookRequest struct {
	Name    string         `json:"name"`
	URL     string         `json:"url"`
	Events  []WebhookEvent `json:"events"`
	Secret  *string        `json:"secret,omitempty"`
	Enabled *bool          `json:"enabled,omitempty"`
}

// Create creates a new outgoing webhook.
func (r *WebhooksResource) Create(ctx context.Context, req *CreateOutgoingWebhookRequest) (*OutgoingWebhook, error) {
	var result OutgoingWebhook
	if err := r.base.Post(ctx, "/api/v1/outgoing-webhooks", req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// Get retrieves a specific outgoing webhook.
func (r *WebhooksResource) Get(ctx context.Context, id string) (*OutgoingWebhook, error) {
	var result OutgoingWebhook
	if err := r.base.Get(ctx, fmt.Sprintf("/api/v1/outgoing-webhooks/%s", id), &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// UpdateOutgoingWebhookRequest is the request to update an outgoing webhook.
type UpdateOutgoingWebhookRequest struct {
	Name    *string         `json:"name,omitempty"`
	URL     *string         `json:"url,omitempty"`
	Events  *[]WebhookEvent `json:"events,omitempty"`
	Secret  *string         `json:"secret,omitempty"`
	Enabled *bool           `json:"enabled,omitempty"`
}

// Update updates an outgoing webhook.
func (r *WebhooksResource) Update(ctx context.Context, id string, req *UpdateOutgoingWebhookRequest) (*OutgoingWebhook, error) {
	var result OutgoingWebhook
	if err := r.base.Put(ctx, fmt.Sprintf("/api/v1/outgoing-webhooks/%s", id), req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// Delete deletes an outgoing webhook.
func (r *WebhooksResource) Delete(ctx context.Context, id string) error {
	return r.base.Delete(ctx, fmt.Sprintf("/api/v1/outgoing-webhooks/%s", id))
}

// TestWebhookResponse is the response from testing a webhook.
type TestWebhookResponse struct {
	Success        bool    `json:"success"`
	StatusCode     *int    `json:"status_code,omitempty"`
	ResponseTimeMs int     `json:"response_time_ms"`
	Error          *string `json:"error,omitempty"`
}

// Test sends a test request to a webhook.
func (r *WebhooksResource) Test(ctx context.Context, id string) (*TestWebhookResponse, error) {
	var result TestWebhookResponse
	if err := r.base.Post(ctx, fmt.Sprintf("/api/v1/outgoing-webhooks/%s/test", id), nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// WebhookDeliveryStatus represents the status of a webhook delivery.
type WebhookDeliveryStatus string

const (
	WebhookDeliveryStatusPending WebhookDeliveryStatus = "pending"
	WebhookDeliveryStatusSuccess WebhookDeliveryStatus = "success"
	WebhookDeliveryStatusFailed  WebhookDeliveryStatus = "failed"
)

// OutgoingWebhookDelivery represents a webhook delivery attempt.
type OutgoingWebhookDelivery struct {
	ID           string                `json:"id"`
	WebhookID    string                `json:"webhook_id"`
	Event        WebhookEvent          `json:"event"`
	Payload      map[string]any        `json:"payload"`
	Status       WebhookDeliveryStatus `json:"status"`
	StatusCode   *int                  `json:"status_code,omitempty"`
	ResponseBody *string               `json:"response_body,omitempty"`
	Error        *string               `json:"error,omitempty"`
	Attempts     int                   `json:"attempts"`
	CreatedAt    time.Time             `json:"created_at"`
	DeliveredAt  *time.Time            `json:"delivered_at,omitempty"`
}

// ListDeliveriesParams are parameters for listing webhook deliveries.
type ListDeliveriesParams struct {
	Status *WebhookDeliveryStatus `json:"status,omitempty"`
	Limit  *int                   `json:"limit,omitempty"`
	Offset *int                   `json:"offset,omitempty"`
}

// Deliveries retrieves delivery attempts for a webhook.
func (r *WebhooksResource) Deliveries(ctx context.Context, id string, params *ListDeliveriesParams) ([]OutgoingWebhookDelivery, error) {
	query := url.Values{}
	if params != nil {
		if params.Status != nil {
			query.Set("status", string(*params.Status))
		}
		AddPaginationParams(query, params.Limit, params.Offset)
	}

	var result []OutgoingWebhookDelivery
	if err := r.base.GetWithQuery(ctx, fmt.Sprintf("/api/v1/outgoing-webhooks/%s/deliveries", id), query, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// RetryDeliveryResponse is the response from retrying a webhook delivery.
type RetryDeliveryResponse struct {
	Success    bool    `json:"success"`
	Message    *string `json:"message,omitempty"`
	Error      *string `json:"error,omitempty"`
}

// RetryDelivery retries a failed webhook delivery.
func (r *WebhooksResource) RetryDelivery(ctx context.Context, webhookID, deliveryID string) (*RetryDeliveryResponse, error) {
	var result RetryDeliveryResponse
	if err := r.base.Post(ctx, fmt.Sprintf("/api/v1/outgoing-webhooks/%s/retry/%s", webhookID, deliveryID), nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}


