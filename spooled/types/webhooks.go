package types

import (
	"encoding/json"
	"errors"
	"time"
)

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
	ID             string         `json:"id"`
	OrganizationID string         `json:"organization_id"`
	Name           string         `json:"name"`
	URL            string         `json:"url"`
	Events         []WebhookEvent `json:"events"`
	// Enabled turns false on its own after 20 consecutive failed deliveries:
	// the server auto-disables the webhook and it stops receiving events.
	// Re-enable it with an update carrying Enabled set to true - that counts
	// against the plan webhook cap, so it can fail with 429 QUOTA_EXCEEDED.
	Enabled bool `json:"enabled"`
	// FailureCount counts consecutive failed deliveries, one per delivery
	// rather than one per retry attempt, so it is roughly five times smaller
	// than a per-attempt count for the same real-world failures. A successful
	// delivery resets it to 0, including a successful manual retry.
	FailureCount    int        `json:"failure_count"`
	LastTriggeredAt *time.Time `json:"last_triggered_at,omitempty"`
	// LastStatus is "success", "failed" or "auto_disabled". The last of those
	// means the webhook hit 20 consecutive failed deliveries and was disabled.
	LastStatus *string   `json:"last_status,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
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
//
// A nil field is left out of the request body and the server keeps the current
// value. The signing secret is the exception: it is three-state, so leaving
// Secret nil keeps it, setting Secret replaces it, and setting ClearSecret
// removes it.
type UpdateOutgoingWebhookRequest struct {
	Name   *string         `json:"name,omitempty"`
	URL    *string         `json:"url,omitempty"`
	Events *[]WebhookEvent `json:"events,omitempty"`
	// Secret replaces the HMAC signing secret. Leave it nil to keep whatever
	// secret the webhook already has.
	Secret *string `json:"secret,omitempty"`
	// ClearSecret sends an explicit null secret, which DELETES the signing
	// secret: deliveries then go out unsigned, with no X-Spooled-Signature
	// header for the receiver to verify. Nothing restores the old value, so
	// set a new Secret to sign again. Setting Secret and ClearSecret together
	// is an error.
	ClearSecret bool `json:"-"`
	// Enabled is how an auto-disabled webhook is brought back: send true after
	// the server disabled it for 20 consecutive failed deliveries. The
	// re-enable counts against the plan webhook cap and can fail with 429
	// QUOTA_EXCEEDED.
	Enabled *bool `json:"enabled,omitempty"`
}

// MarshalJSON encodes the update request, turning ClearSecret into the explicit
// JSON null that removes the signing secret. Without it the secret field is
// omitted entirely, which is what keeps the current secret - a nil *string
// alone can never express "clear this".
func (r UpdateOutgoingWebhookRequest) MarshalJSON() ([]byte, error) {
	type payload UpdateOutgoingWebhookRequest

	if !r.ClearSecret {
		return json.Marshal(payload(r))
	}
	if r.Secret != nil {
		return nil, errors.New("spooled: UpdateOutgoingWebhookRequest sets both Secret and ClearSecret; set exactly one")
	}

	body, err := json.Marshal(payload(r))
	if err != nil {
		return nil, err
	}

	fields := map[string]json.RawMessage{}
	if err := json.Unmarshal(body, &fields); err != nil {
		return nil, err
	}
	fields["secret"] = json.RawMessage("null")
	return json.Marshal(fields)
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
