package resources

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/spooled-cloud/spooled-sdk-go/internal/httpx"
)

// IngestResource provides access to webhook ingestion operations.
type IngestResource struct {
	base *Base
}

// NewIngestResource creates a new IngestResource.
func NewIngestResource(transport *httpx.Transport) *IngestResource {
	return &IngestResource{base: NewBase(transport)}
}

// ErrWebhookTokenRequired is returned by Ingest().Custom when the request has no
// webhook token. The API authenticates POST /webhooks/{org_id}/custom with the
// organization's webhook token (X-Webhook-Token) and answers every call without
// a valid one with 401, so the call is refused before it reaches the network.
var ErrWebhookTokenRequired = errors.New("spooled: webhook ingestion needs the organization's webhook token " +
	"(set CustomWebhookRequest.WebhookToken, or use CustomWithToken; Organizations().GetWebhookToken returns it)")

// CustomWebhookRequest is the request to ingest a custom webhook.
type CustomWebhookRequest struct {
	QueueName      string         `json:"queue_name"`
	EventType      *string        `json:"event_type,omitempty"`
	Payload        map[string]any `json:"payload"`
	IdempotencyKey *string        `json:"idempotency_key,omitempty"`
	Priority       *int           `json:"priority,omitempty"`
	// WebhookToken is the organization's webhook token, sent as the
	// X-Webhook-Token header (never in the body). Ingest().Custom requires it;
	// CustomWithToken takes the token as an argument instead.
	WebhookToken string `json:"-"`
}

// CustomWebhookResponse is POST /webhooks/{org_id}/custom (OpenAPI WebhookResponse).
type CustomWebhookResponse struct {
	JobID     string `json:"job_id"`
	QueueName string `json:"queue_name"`
	Status    string `json:"status"`
}

// Custom ingests a custom webhook for an organization, authenticating with
// req.WebhookToken (the X-Webhook-Token header).
//
// POST /webhooks/{org_id}/custom returns {job_id, queue_name, status}. An
// older empty 200 maps to a zero response rather than an error. Without a
// token it returns ErrWebhookTokenRequired and makes no request: the API
// rejects an unauthenticated ingest with 401 unconditionally.
func (r *IngestResource) Custom(ctx context.Context, orgID string, req *CustomWebhookRequest) (*CustomWebhookResponse, error) {
	if req == nil || req.WebhookToken == "" {
		return nil, ErrWebhookTokenRequired
	}
	return r.CustomWithToken(ctx, orgID, req.WebhookToken, req)
}

// CustomWithToken ingests a custom webhook using a webhook token via the X-Webhook-Token header.
func (r *IngestResource) CustomWithToken(ctx context.Context, orgID, webhookToken string, req *CustomWebhookRequest) (*CustomWebhookResponse, error) {
	resp, err := r.base.transport.Do(ctx, &httpx.Request{
		Method: http.MethodPost,
		Path:   fmt.Sprintf("/api/v1/webhooks/%s/custom", orgID),
		Body:   req,
		Headers: map[string]string{
			"X-Webhook-Token": webhookToken,
		},
	})
	if err != nil {
		return nil, err
	}
	out, err := httpx.JSON[CustomWebhookResponse](resp)
	if err != nil {
		return nil, err
	}
	if out == nil {
		return &CustomWebhookResponse{}, nil
	}
	return out, nil
}

// InboundDeliveryStatus says what an accepted inbound webhook request did.
//
// It is a string type: a status the API adds later decodes as its name.
type InboundDeliveryStatus string

const (
	// InboundDeliveryCreated: the request created a new job.
	InboundDeliveryCreated InboundDeliveryStatus = "created"
	// InboundDeliveryDuplicate: the request's idempotency key matched an
	// existing job, so no job was created.
	InboundDeliveryDuplicate InboundDeliveryStatus = "duplicate"
)

// InboundWebhookDelivery is one accepted request to
// POST /webhooks/{org_id}/custom, as the inbound webhook log lists it. The log
// keeps neither the request payload nor any signature.
type InboundWebhookDelivery struct {
	ID        string `json:"id"`
	Provider  string `json:"provider"`
	EventType string `json:"event_type"`
	// MatchedQueue is the queue the job went to.
	MatchedQueue *string `json:"matched_queue,omitempty"`
	// JobID is the job the request created or matched. Nil for entries
	// recorded before API 0.1.122.
	JobID *string `json:"job_id,omitempty"`
	// Status is InboundDeliveryCreated or InboundDeliveryDuplicate. Nil for
	// entries recorded before API 0.1.122.
	Status    *InboundDeliveryStatus `json:"status,omitempty"`
	CreatedAt time.Time              `json:"created_at"`
}

// ListInboundDeliveriesParams are parameters for Ingest().ListDeliveries.
type ListInboundDeliveriesParams struct {
	// Status keeps entries with this status. An unknown value is a 400.
	Status *InboundDeliveryStatus `json:"status,omitempty"`
	// Since keeps entries received at or after this instant.
	Since *time.Time `json:"since,omitempty"`
	// QueueName keeps entries whose job went to this queue.
	QueueName *string `json:"queue_name,omitempty"`
	// Limit is the page size, 1-100 (API default 50).
	Limit  *int `json:"limit,omitempty"`
	Offset *int `json:"offset,omitempty"`
}

// ListDeliveries returns the inbound webhook log
// (GET /webhooks/deliveries): the requests to POST /webhooks/{org_id}/custom
// the API accepted, newest first. The log is organization-wide, so an API key
// restricted to some queues is refused (403).
func (r *IngestResource) ListDeliveries(ctx context.Context, params *ListInboundDeliveriesParams) ([]InboundWebhookDelivery, error) {
	query := url.Values{}
	if params != nil {
		if params.Status != nil {
			query.Set("status", string(*params.Status))
		}
		setTimeParam(query, "since", params.Since)
		if params.QueueName != nil {
			query.Set("queue_name", *params.QueueName)
		}
		AddPaginationParams(query, params.Limit, params.Offset)
	}
	var result []InboundWebhookDelivery
	if err := r.base.GetWithQuery(ctx, "/api/v1/webhooks/deliveries", query, &result); err != nil {
		return nil, err
	}
	if result == nil {
		result = []InboundWebhookDelivery{}
	}
	return result, nil
}

// GitHubIngestOptions configures GitHub webhook ingestion.
//
// Deprecated: the Spooled API has no GitHub ingestion route.
type GitHubIngestOptions struct {
	// GitHubEvent is the GitHub event name, e.g. "push".
	GitHubEvent string
	// Signature is the X-Hub-Signature-256 value.
	Signature string
	// Secret was used to compute the signature.
	Secret string
	// WebhookToken is sent as X-Webhook-Token.
	WebhookToken string
	// ForwardedProto is sent as X-Forwarded-Proto.
	ForwardedProto string
}

// StripeIngestOptions configures Stripe webhook ingestion.
//
// Deprecated: the Spooled API has no Stripe ingestion route.
type StripeIngestOptions struct {
	// Timestamp used for the signature payload.
	Timestamp int64
	// Signature is the Stripe-Signature header value.
	Signature string
	// Secret was used to compute the signature.
	Secret string
	// WebhookToken is sent as X-Webhook-Token.
	WebhookToken string
	// ForwardedProto is sent as X-Forwarded-Proto.
	ForwardedProto string
}

// GitHub always returns an UnsupportedError and makes no request.
//
// Deprecated: the API serves only POST /webhooks/{org_id}/custom. Its GitHub
// and Stripe handlers were removed server side, so this call used to get a 404
// every time. Forward GitHub events to Custom/CustomWithToken (the JSON body
// goes in Payload), or enqueue the job yourself with Jobs().Create.
func (r *IngestResource) GitHub(_ context.Context, _ string, _ []byte, _ *GitHubIngestOptions) error {
	return &UnsupportedError{
		Operation: "Ingest.GitHub",
		Reason:    "the API serves only POST /webhooks/{org_id}/custom; forward the event with CustomWithToken or enqueue it with Jobs().Create",
	}
}

// Stripe always returns an UnsupportedError and makes no request.
//
// Deprecated: the API serves only POST /webhooks/{org_id}/custom (Stripe
// billing events are handled internally at /billing/webhook). Forward the event
// to Custom/CustomWithToken or enqueue the job yourself with Jobs().Create.
func (r *IngestResource) Stripe(_ context.Context, _ string, _ []byte, _ *StripeIngestOptions) error {
	return &UnsupportedError{
		Operation: "Ingest.Stripe",
		Reason:    "the API serves only POST /webhooks/{org_id}/custom; forward the event with CustomWithToken or enqueue it with Jobs().Create",
	}
}
