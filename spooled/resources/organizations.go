package resources

import (
	"context"
	"fmt"
	"time"

	"github.com/spooled-cloud/spooled-sdk-go/internal/httpx"
)

// OrganizationsResource provides access to organization operations.
type OrganizationsResource struct {
	base *Base
}

// NewOrganizationsResource creates a new OrganizationsResource.
func NewOrganizationsResource(transport *httpx.Transport) *OrganizationsResource {
	return &OrganizationsResource{base: NewBase(transport)}
}

// PlanTier represents a subscription plan tier.
type PlanTier string

const (
	PlanTierFree       PlanTier = "free"
	PlanTierStarter    PlanTier = "starter"
	PlanTierPro        PlanTier = "pro"
	PlanTierEnterprise PlanTier = "enterprise"
)

// Organization represents an organization.
type Organization struct {
	ID                       string         `json:"id"`
	Name                     string         `json:"name"`
	Slug                     string         `json:"slug"`
	PlanTier                 PlanTier       `json:"plan_tier"`
	BillingEmail             *string        `json:"billing_email,omitempty"`
	Settings                 map[string]any `json:"settings"`
	CustomLimits             map[string]any `json:"custom_limits,omitempty"`
	StripeCustomerID         *string        `json:"stripe_customer_id,omitempty"`
	StripeSubscriptionID     *string        `json:"stripe_subscription_id,omitempty"`
	StripeSubscriptionStatus *string        `json:"stripe_subscription_status,omitempty"`
	StripeCurrentPeriodEnd   *time.Time     `json:"stripe_current_period_end,omitempty"`
	StripeCancelAtPeriodEnd  *bool          `json:"stripe_cancel_at_period_end,omitempty"`
	CreatedAt                time.Time      `json:"created_at"`
	UpdatedAt                time.Time      `json:"updated_at"`
}

// List retrieves all organizations for the current user.
func (r *OrganizationsResource) List(ctx context.Context) ([]Organization, error) {
	var result []Organization
	if err := r.base.Get(ctx, "/api/v1/organizations", &result); err != nil {
		return nil, err
	}
	return result, nil
}

// CreateOrganizationRequest is the request to create an organization.
type CreateOrganizationRequest struct {
	Name         string  `json:"name"`
	Slug         string  `json:"slug"`
	BillingEmail *string `json:"billing_email,omitempty"`
}

// CreateOrganizationResponse is the response from creating an organization.
type CreateOrganizationResponse struct {
	Organization Organization         `json:"organization"`
	APIKey       CreateAPIKeyResponse `json:"api_key"`
}

// Create creates a new organization.
func (r *OrganizationsResource) Create(ctx context.Context, req *CreateOrganizationRequest) (*CreateOrganizationResponse, error) {
	var result CreateOrganizationResponse
	if err := r.base.Post(ctx, "/api/v1/organizations", req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// Get retrieves a specific organization.
func (r *OrganizationsResource) Get(ctx context.Context, id string) (*Organization, error) {
	var result Organization
	if err := r.base.Get(ctx, fmt.Sprintf("/api/v1/organizations/%s", id), &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// UpdateOrganizationRequest is the request to update an organization.
type UpdateOrganizationRequest struct {
	Name         *string        `json:"name,omitempty"`
	BillingEmail *string        `json:"billing_email,omitempty"`
	Settings     map[string]any `json:"settings,omitempty"`
}

// Update updates an organization.
func (r *OrganizationsResource) Update(ctx context.Context, id string, req *UpdateOrganizationRequest) (*Organization, error) {
	var result Organization
	if err := r.base.Put(ctx, fmt.Sprintf("/api/v1/organizations/%s", id), req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// Delete deletes an organization.
func (r *OrganizationsResource) Delete(ctx context.Context, id string) error {
	return r.base.Delete(ctx, fmt.Sprintf("/api/v1/organizations/%s", id))
}

// UsageInfo represents organization usage information.
type UsageInfo struct {
	Plan            string         `json:"plan"`
	PlanDisplayName string         `json:"plan_display_name"`
	Limits          PlanLimits     `json:"limits"`
	Usage           ResourceUsage  `json:"usage"`
	Warnings        []UsageWarning `json:"warnings,omitempty"`
}

// PlanLimits represents the limits for a plan.
type PlanLimits struct {
	Tier                       string `json:"tier"`
	DisplayName                string `json:"display_name"`
	MaxJobsPerDay              *int   `json:"max_jobs_per_day,omitempty"`
	MaxActiveJobs              *int   `json:"max_active_jobs,omitempty"`
	MaxQueues                  *int   `json:"max_queues,omitempty"`
	MaxWorkers                 *int   `json:"max_workers,omitempty"`
	MaxAPIKeys                 *int   `json:"max_api_keys,omitempty"`
	MaxSchedules               *int   `json:"max_schedules,omitempty"`
	MaxWorkflows               *int   `json:"max_workflows,omitempty"`
	MaxWebhooks                *int   `json:"max_webhooks,omitempty"`
	MaxPayloadSizeBytes        int    `json:"max_payload_size_bytes"`
	RateLimitRequestsPerSecond int    `json:"rate_limit_requests_per_second"`
	RateLimitBurst             int    `json:"rate_limit_burst"`
	JobRetentionDays           int    `json:"job_retention_days"`
	HistoryRetentionDays       int    `json:"history_retention_days"`
}

// ResourceUsage represents current resource usage.
type ResourceUsage struct {
	JobsToday  UsageItem `json:"jobs_today"`
	ActiveJobs UsageItem `json:"active_jobs"`
	Queues     UsageItem `json:"queues"`
	Workers    UsageItem `json:"workers"`
	APIKeys    UsageItem `json:"api_keys"`
	Schedules  UsageItem `json:"schedules"`
	Workflows  UsageItem `json:"workflows"`
	Webhooks   UsageItem `json:"webhooks"`
}

// UsageItem represents usage for a single resource type.
type UsageItem struct {
	Current    int      `json:"current"`
	Limit      *int     `json:"limit,omitempty"`
	Percentage *float64 `json:"percentage,omitempty"`
	IsDisabled bool     `json:"is_disabled"`
}

// UsageWarning represents a usage warning.
type UsageWarning struct {
	Resource string `json:"resource"`
	Message  string `json:"message"`
	Severity string `json:"severity"`
}

// Usage retrieves usage information for an organization.
func (r *OrganizationsResource) Usage(ctx context.Context, id string) (*UsageInfo, error) {
	var result UsageInfo
	if err := r.base.Get(ctx, fmt.Sprintf("/api/v1/organizations/%s/usage", id), &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// OrganizationMember represents a member of an organization.
type OrganizationMember struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Email     string    `json:"email"`
	Name      string    `json:"name"`
	Role      string    `json:"role"`
	JoinedAt  time.Time `json:"joined_at"`
	InvitedBy *string   `json:"invited_by,omitempty"`
}

// Members retrieves members of an organization.
func (r *OrganizationsResource) Members(ctx context.Context, id string) ([]OrganizationMember, error) {
	var result []OrganizationMember
	if err := r.base.Get(ctx, fmt.Sprintf("/api/v1/organizations/%s/members", id), &result); err != nil {
		return nil, err
	}
	return result, nil
}

// CheckSlugResponse is the response from checking slug availability.
type CheckSlugResponse struct {
	Available bool    `json:"available"`
	Slug      string  `json:"slug"`
	Message   *string `json:"message,omitempty"`
}

// CheckSlug checks if a slug is available.
func (r *OrganizationsResource) CheckSlug(ctx context.Context, slug string) (*CheckSlugResponse, error) {
	var result CheckSlugResponse
	if err := r.base.Get(ctx, fmt.Sprintf("/api/v1/organizations/check-slug/%s", slug), &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GenerateSlugResponse is the response from generating a slug.
type GenerateSlugResponse struct {
	Slug string `json:"slug"`
}

// GenerateSlug generates a slug from a name.
func (r *OrganizationsResource) GenerateSlug(ctx context.Context, name string) (*GenerateSlugResponse, error) {
	var result GenerateSlugResponse
	if err := r.base.Post(ctx, "/api/v1/organizations/generate-slug", map[string]string{"name": name}, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// WebhookTokenResponse is the response for webhook token operations.
type WebhookTokenResponse struct {
	WebhookToken *string `json:"webhook_token,omitempty"`
	WebhookURL   *string `json:"webhook_url,omitempty"`
}

// GetWebhookToken retrieves the webhook token for an organization.
func (r *OrganizationsResource) GetWebhookToken(ctx context.Context, id string) (*WebhookTokenResponse, error) {
	var result WebhookTokenResponse
	if err := r.base.Get(ctx, fmt.Sprintf("/api/v1/organizations/%s/webhook-token", id), &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// RegenerateWebhookToken regenerates the webhook token for an organization.
func (r *OrganizationsResource) RegenerateWebhookToken(ctx context.Context, id string) (*WebhookTokenResponse, error) {
	var result WebhookTokenResponse
	if err := r.base.Post(ctx, fmt.Sprintf("/api/v1/organizations/%s/webhook-token/regenerate", id), nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// ClearWebhookToken clears the webhook token for an organization.
func (r *OrganizationsResource) ClearWebhookToken(ctx context.Context, id string) error {
	return r.base.Delete(ctx, fmt.Sprintf("/api/v1/organizations/%s/webhook-token", id))
}
