package resources

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
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

// CustomWebhookRequest is the request to ingest a custom webhook.
type CustomWebhookRequest struct {
	QueueName      string         `json:"queue_name"`
	EventType      *string        `json:"event_type,omitempty"`
	Payload        map[string]any `json:"payload"`
	IdempotencyKey *string        `json:"idempotency_key,omitempty"`
	Priority       *int           `json:"priority,omitempty"`
}

// CustomWebhookResponse is the response from ingesting a custom webhook.
type CustomWebhookResponse struct {
	JobID   string `json:"job_id"`
	Created bool   `json:"created"`
}

// Custom ingests a custom webhook for an organization.
func (r *IngestResource) Custom(ctx context.Context, orgID string, req *CustomWebhookRequest) (*CustomWebhookResponse, error) {
	var result CustomWebhookResponse
	if err := r.base.Post(ctx, fmt.Sprintf("/api/v1/webhooks/%s/custom", orgID), req, &result); err != nil {
		return nil, err
	}
	return &result, nil
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
		return nil, fmt.Errorf("empty response")
	}
	return out, nil
}

// GitHubIngestOptions configures GitHub webhook ingestion.
type GitHubIngestOptions struct {
	// GitHubEvent is the GitHub event name (required), e.g. "push".
	GitHubEvent string
	// Signature is the full X-Hub-Signature-256 value (optional if Secret is provided).
	Signature string
	// Secret is used to compute the signature if Signature is not provided.
	Secret string
	// WebhookToken is optional and sent as X-Webhook-Token.
	WebhookToken string
	// ForwardedProto is optional and sent as X-Forwarded-Proto.
	ForwardedProto string
}

// StripeIngestOptions configures Stripe webhook ingestion.
type StripeIngestOptions struct {
	// Timestamp used for signature payload. If 0, current time is used.
	Timestamp int64
	// Signature is the full Stripe-Signature header value (optional if Secret is provided).
	Signature string
	// Secret is used to compute the signature if Signature is not provided.
	Secret string
	// WebhookToken is optional and sent as X-Webhook-Token.
	WebhookToken string
	// ForwardedProto is optional and sent as X-Forwarded-Proto.
	ForwardedProto string
}

// GitHub ingests a GitHub webhook. The request body must be the exact raw bytes GitHub sent.
// POST /api/v1/webhooks/{org_id}/github
func (r *IngestResource) GitHub(ctx context.Context, orgID string, rawBody []byte, opts *GitHubIngestOptions) error {
	if opts == nil {
		return fmt.Errorf("GitHub ingest options are required")
	}
	if opts.GitHubEvent == "" {
		return fmt.Errorf("GitHubEvent is required")
	}

	signature := opts.Signature
	if signature == "" && opts.Secret != "" {
		signature = githubSignature(opts.Secret, rawBody)
	}
	if signature == "" {
		return fmt.Errorf("GitHub signature is required (provide Signature or Secret)")
	}

	headers := map[string]string{
		"Content-Type":        "application/json",
		"X-GitHub-Event":      opts.GitHubEvent,
		"X-Hub-Signature-256": signature,
	}
	if opts.WebhookToken != "" {
		headers["X-Webhook-Token"] = opts.WebhookToken
	}
	if opts.ForwardedProto != "" {
		headers["X-Forwarded-Proto"] = opts.ForwardedProto
	}

	_, err := r.base.transport.Do(ctx, &httpx.Request{
		Method:  http.MethodPost,
		Path:    fmt.Sprintf("/api/v1/webhooks/%s/github", orgID),
		RawBody: rawBody,
		Headers: headers,
	})
	return err
}

// Stripe ingests a Stripe webhook. The request body must be the exact raw bytes Stripe sent.
// POST /api/v1/webhooks/{org_id}/stripe
func (r *IngestResource) Stripe(ctx context.Context, orgID string, rawBody []byte, opts *StripeIngestOptions) error {
	if opts == nil {
		opts = &StripeIngestOptions{}
	}

	timestamp := opts.Timestamp
	if timestamp == 0 {
		timestamp = time.Now().Unix()
	}

	signature := opts.Signature
	if signature == "" && opts.Secret != "" {
		signature = stripeSignature(opts.Secret, rawBody, timestamp)
	}
	if signature == "" {
		return fmt.Errorf("Stripe signature is required (provide Signature or Secret)")
	}

	headers := map[string]string{
		"Content-Type":     "application/json",
		"Stripe-Signature": signature,
	}
	if opts.WebhookToken != "" {
		headers["X-Webhook-Token"] = opts.WebhookToken
	}
	if opts.ForwardedProto != "" {
		headers["X-Forwarded-Proto"] = opts.ForwardedProto
	}

	_, err := r.base.transport.Do(ctx, &httpx.Request{
		Method:  http.MethodPost,
		Path:    fmt.Sprintf("/api/v1/webhooks/%s/stripe", orgID),
		RawBody: rawBody,
		Headers: headers,
	})
	return err
}

func githubSignature(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func stripeSignature(secret string, body []byte, timestamp int64) string {
	// Stripe signs: `${timestamp}.${rawBodyUtf8}`
	payload := strconv.FormatInt(timestamp, 10) + "." + string(body)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(payload))
	return "t=" + strconv.FormatInt(timestamp, 10) + ",v1=" + hex.EncodeToString(mac.Sum(nil))
}


