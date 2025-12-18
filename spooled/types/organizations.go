package types

import "time"

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
	ID                       string      `json:"id"`
	Name                     string      `json:"name"`
	Slug                     string      `json:"slug"`
	PlanTier                 PlanTier    `json:"plan_tier"`
	BillingEmail             *string     `json:"billing_email,omitempty"`
	Settings                 JsonObject  `json:"settings"`
	CustomLimits             *JsonObject `json:"custom_limits,omitempty"`
	StripeCustomerID         *string     `json:"stripe_customer_id,omitempty"`
	StripeSubscriptionID     *string     `json:"stripe_subscription_id,omitempty"`
	StripeSubscriptionStatus *string     `json:"stripe_subscription_status,omitempty"`
	StripeCurrentPeriodEnd   *time.Time  `json:"stripe_current_period_end,omitempty"`
	StripeCancelAtPeriodEnd  *bool       `json:"stripe_cancel_at_period_end,omitempty"`
	CreatedAt                time.Time   `json:"created_at"`
	UpdatedAt                time.Time   `json:"updated_at"`
}

// OrganizationSummary is a summary of an organization.
type OrganizationSummary struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Slug      string    `json:"slug"`
	PlanTier  PlanTier  `json:"plan_tier"`
	CreatedAt time.Time `json:"created_at"`
}

// CreateOrganizationRequest is the request to create an organization.
type CreateOrganizationRequest struct {
	Name         string  `json:"name"`
	Slug         string  `json:"slug"`
	BillingEmail *string `json:"billing_email,omitempty"`
}

// CreateOrganizationResponse is the response from creating an organization.
type CreateOrganizationResponse struct {
	Organization Organization          `json:"organization"`
	APIKey       CreateAPIKeyResponse  `json:"api_key"`
}

// UpdateOrganizationRequest is the request to update an organization.
type UpdateOrganizationRequest struct {
	Name         *string     `json:"name,omitempty"`
	BillingEmail *string     `json:"billing_email,omitempty"`
	Settings     *JsonObject `json:"settings,omitempty"`
}

// UsageInfo represents organization usage information.
type UsageInfo struct {
	Plan            string        `json:"plan"`
	PlanDisplayName string        `json:"plan_display_name"`
	Limits          PlanLimits    `json:"limits"`
	Usage           ResourceUsage `json:"usage"`
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
	Severity string `json:"severity"` // "warning" or "critical"
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

// CheckSlugResponse is the response from checking slug availability.
type CheckSlugResponse struct {
	Available bool    `json:"available"`
	Slug      string  `json:"slug"`
	Message   *string `json:"message,omitempty"`
}

// GenerateSlugResponse is the response from generating a slug.
type GenerateSlugResponse struct {
	Slug string `json:"slug"`
}

// WebhookTokenResponse is the response for webhook token operations.
type WebhookTokenResponse struct {
	WebhookToken *string `json:"webhook_token,omitempty"`
	WebhookURL   *string `json:"webhook_url,omitempty"`
}


