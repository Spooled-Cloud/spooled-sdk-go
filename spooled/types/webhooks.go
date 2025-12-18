package types

import "time"

// WebhookEvent represents a webhook event type.
type WebhookEvent string

const (
	WebhookEventJobCreated          WebhookEvent = "job.created"
	WebhookEventJobStarted          WebhookEvent = "job.started"
	WebhookEventJobCompleted        WebhookEvent = "job.completed"
	WebhookEventJobFailed           WebhookEvent = "job.failed"
	WebhookEventJobCancelled        WebhookEvent = "job.cancelled"
	WebhookEventQueuePaused         WebhookEvent = "queue.paused"
	WebhookEventQueueResumed        WebhookEvent = "queue.resumed"
	WebhookEventWorkerRegistered    WebhookEvent = "worker.registered"
	WebhookEventWorkerDeregistered  WebhookEvent = "worker.deregistered"
	WebhookEventScheduleTriggered   WebhookEvent = "schedule.triggered"
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

// CreateOutgoingWebhookRequest is the request to create an outgoing webhook.
type CreateOutgoingWebhookRequest struct {
	Name    string         `json:"name"`
	URL     string         `json:"url"`
	Events  []WebhookEvent `json:"events"`
	Secret  *string        `json:"secret,omitempty"`
	Enabled *bool          `json:"enabled,omitempty"`
}

// UpdateOutgoingWebhookRequest is the request to update an outgoing webhook.
type UpdateOutgoingWebhookRequest struct {
	Name    *string         `json:"name,omitempty"`
	URL     *string         `json:"url,omitempty"`
	Events  *[]WebhookEvent `json:"events,omitempty"`
	Secret  *string         `json:"secret,omitempty"`
	Enabled *bool           `json:"enabled,omitempty"`
}

// TestWebhookResponse is the response from testing a webhook.
type TestWebhookResponse struct {
	Success        bool    `json:"success"`
	StatusCode     *int    `json:"status_code,omitempty"`
	ResponseTimeMs int     `json:"response_time_ms"`
	Error          *string `json:"error,omitempty"`
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
	Payload      JsonObject            `json:"payload"`
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

// RetryDeliveryResponse is the response from retrying a webhook delivery.
type RetryDeliveryResponse struct {
	Success    bool    `json:"success"`
	DeliveryID string  `json:"delivery_id"`
	Error      *string `json:"error,omitempty"`
}


