package types

import "time"

// AdminStats contains platform-wide statistics.
type AdminStats struct {
	TotalOrganizations int                `json:"total_organizations"`
	TotalJobs          int                `json:"total_jobs"`
	TotalWorkers       int                `json:"total_workers"`
	TotalQueues        int                `json:"total_queues"`
	TotalAPIKeys       int                `json:"total_api_keys"`
	JobsByStatus       map[string]int     `json:"jobs_by_status"`
	OrganizationsByPlan map[string]int    `json:"organizations_by_plan"`
}

// PlanInfo contains information about a subscription plan.
type PlanInfo struct {
	Tier        PlanTier   `json:"tier"`
	DisplayName string     `json:"display_name"`
	Description string     `json:"description"`
	Limits      PlanLimits `json:"limits"`
	Price       *PlanPrice `json:"price,omitempty"`
}

// PlanPrice represents pricing information for a plan.
type PlanPrice struct {
	MonthlyUSD int    `json:"monthly_usd"`
	YearlyUSD  int    `json:"yearly_usd"`
	Currency   string `json:"currency"`
}

// AdminListOrganizationsParams are parameters for admin listing organizations.
type AdminListOrganizationsParams struct {
	PlanTier  *PlanTier `json:"plan_tier,omitempty"`
	Search    *string   `json:"search,omitempty"`
	Limit     *int      `json:"limit,omitempty"`
	Offset    *int      `json:"offset,omitempty"`
}

// AdminCreateOrganizationRequest is the admin request to create an organization.
type AdminCreateOrganizationRequest struct {
	Name         string      `json:"name"`
	Slug         string      `json:"slug"`
	PlanTier     *PlanTier   `json:"plan_tier,omitempty"`
	BillingEmail *string     `json:"billing_email,omitempty"`
	CustomLimits *JsonObject `json:"custom_limits,omitempty"`
}

// AdminUpdateOrganizationRequest is the admin request to update an organization.
type AdminUpdateOrganizationRequest struct {
	Name         *string     `json:"name,omitempty"`
	PlanTier     *PlanTier   `json:"plan_tier,omitempty"`
	BillingEmail *string     `json:"billing_email,omitempty"`
	CustomLimits *JsonObject `json:"custom_limits,omitempty"`
	IsActive     *bool       `json:"is_active,omitempty"`
}

// AdminDeleteOrganizationParams are parameters for admin deleting an organization.
type AdminDeleteOrganizationParams struct {
	Hard bool `json:"hard"` // If true, performs hard delete
}

// AdminCreateAPIKeyRequest is the admin request to create an API key for an org.
type AdminCreateAPIKeyRequest struct {
	Name      string     `json:"name"`
	Queues    []string   `json:"queues,omitempty"`
	RateLimit *int       `json:"rate_limit,omitempty"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

// IngestRequest is the request to ingest a custom webhook.
type CustomWebhookRequest struct {
	QueueName      string     `json:"queue_name"`
	EventType      *string    `json:"event_type,omitempty"`
	Payload        JsonObject `json:"payload"`
	IdempotencyKey *string    `json:"idempotency_key,omitempty"`
	Priority       *int       `json:"priority,omitempty"`
}

// CustomWebhookResponse is the response from ingesting a custom webhook.
type CustomWebhookResponse struct {
	JobID   string `json:"job_id"`
	Created bool   `json:"created"`
}


