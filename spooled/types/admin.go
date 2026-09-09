package types

import "time"

// AdminPlanCount is one plan's organization count from GET /admin/stats.
type AdminPlanCount struct {
	Plan  string `json:"plan"`
	Count int    `json:"count"`
}

// AdminOrgStats is organization counts from GET /admin/stats.
type AdminOrgStats struct {
	Total           int              `json:"total"`
	ByPlan          []AdminPlanCount `json:"by_plan"`
	CreatedToday    int              `json:"created_today"`
	CreatedThisWeek int              `json:"created_this_week"`
}

// AdminJobStats is job counts from GET /admin/stats.
type AdminJobStats struct {
	TotalActive  int `json:"total_active"`
	Pending      int `json:"pending"`
	Processing   int `json:"processing"`
	Completed24h int `json:"completed_24h"`
	Failed24h    int `json:"failed_24h"`
}

// AdminWorkerStats is worker counts from GET /admin/stats.
type AdminWorkerStats struct {
	Total    int `json:"total"`
	Healthy  int `json:"healthy"`
	Degraded int `json:"degraded"`
}

// AdminSystemStats is process info from GET /admin/stats.
type AdminSystemStats struct {
	APIVersion    string `json:"api_version"`
	UptimeSeconds int64  `json:"uptime_seconds"`
}

// AdminStats is GET /admin/stats — nested org/job/worker/system counts, not flat totals.
type AdminStats struct {
	Organizations AdminOrgStats    `json:"organizations"`
	Jobs          AdminJobStats    `json:"jobs"`
	Workers       AdminWorkerStats `json:"workers"`
	System        AdminSystemStats `json:"system"`
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
	PlanTier *PlanTier `json:"plan_tier,omitempty"`
	Search   *string   `json:"search,omitempty"`
	Limit    *int      `json:"limit,omitempty"`
	Offset   *int      `json:"offset,omitempty"`
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
