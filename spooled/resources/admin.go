package resources

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/spooled-cloud/spooled-sdk-go/internal/httpx"
)

// AdminResource provides access to admin operations.
// All operations require the X-Admin-Key header.
type AdminResource struct {
	base *Base
}

// NewAdminResource creates a new AdminResource.
func NewAdminResource(transport *httpx.Transport) *AdminResource {
	return &AdminResource{base: NewBase(transport)}
}

// AdminStats contains platform-wide statistics.
type AdminStats struct {
	TotalOrganizations  int            `json:"total_organizations"`
	TotalJobs           int            `json:"total_jobs"`
	TotalWorkers        int            `json:"total_workers"`
	TotalQueues         int            `json:"total_queues"`
	TotalAPIKeys        int            `json:"total_api_keys"`
	JobsByStatus        map[string]int `json:"jobs_by_status"`
	OrganizationsByPlan map[string]int `json:"organizations_by_plan"`
}

// GetStats retrieves platform-wide statistics.
func (r *AdminResource) GetStats(ctx context.Context) (*AdminStats, error) {
	var result AdminStats
	if err := r.base.AdminGet(ctx, "/api/v1/admin/stats", &result); err != nil {
		return nil, err
	}
	return &result, nil
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

// GetPlans retrieves all available subscription plans.
func (r *AdminResource) GetPlans(ctx context.Context) ([]PlanInfo, error) {
	var result []PlanInfo
	if err := r.base.AdminGet(ctx, "/api/v1/admin/plans", &result); err != nil {
		return nil, err
	}
	return result, nil
}

// ListOrganizationsParams are parameters for admin listing organizations.
type ListOrganizationsParams struct {
	PlanTier *PlanTier `json:"plan_tier,omitempty"`
	Search   *string   `json:"search,omitempty"`
	Limit    *int      `json:"limit,omitempty"`
	Offset   *int      `json:"offset,omitempty"`
}

// AdminOrganizationList is the paginated response for admin list organizations.
type AdminOrganizationList struct {
	Organizations []Organization `json:"organizations"`
	Total         int            `json:"total"`
	Limit         int            `json:"limit"`
	Offset        int            `json:"offset"`
}

// ListOrganizations retrieves all organizations (admin only).
func (r *AdminResource) ListOrganizations(ctx context.Context, params *ListOrganizationsParams) (*AdminOrganizationList, error) {
	query := url.Values{}
	if params != nil {
		if params.PlanTier != nil {
			query.Set("plan_tier", string(*params.PlanTier))
		}
		if params.Search != nil {
			query.Set("search", *params.Search)
		}
		AddPaginationParams(query, params.Limit, params.Offset)
	}

	var result AdminOrganizationList
	// Use AdminGet but we need to add query params
	path := "/api/v1/admin/organizations"
	if len(query) > 0 {
		path += "?" + query.Encode()
	}
	if err := r.base.AdminGet(ctx, path, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// CreateOrganizationRequest is the admin request to create an organization.
type AdminCreateOrganizationRequest struct {
	Name         string         `json:"name"`
	Slug         string         `json:"slug"`
	PlanTier     *PlanTier      `json:"plan_tier,omitempty"`
	BillingEmail *string        `json:"billing_email,omitempty"`
	CustomLimits map[string]any `json:"custom_limits,omitempty"`
}

// CreateOrganization creates an organization (admin only).
func (r *AdminResource) CreateOrganization(ctx context.Context, req *AdminCreateOrganizationRequest) (*CreateOrganizationResponse, error) {
	var result CreateOrganizationResponse
	if err := r.base.AdminPost(ctx, "/api/v1/admin/organizations", req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetOrganization retrieves a specific organization (admin only).
func (r *AdminResource) GetOrganization(ctx context.Context, id string) (*Organization, error) {
	var result Organization
	if err := r.base.AdminGet(ctx, fmt.Sprintf("/api/v1/admin/organizations/%s", id), &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// UpdateOrganizationRequest is the admin request to update an organization.
type AdminUpdateOrganizationRequest struct {
	Name         *string        `json:"name,omitempty"`
	PlanTier     *PlanTier      `json:"plan_tier,omitempty"`
	BillingEmail *string        `json:"billing_email,omitempty"`
	CustomLimits map[string]any `json:"custom_limits,omitempty"`
	IsActive     *bool          `json:"is_active,omitempty"`
}

// UpdateOrganization updates an organization (admin only).
func (r *AdminResource) UpdateOrganization(ctx context.Context, id string, req *AdminUpdateOrganizationRequest) (*Organization, error) {
	var result Organization
	if err := r.base.AdminPut(ctx, fmt.Sprintf("/api/v1/admin/organizations/%s", id), req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// DeleteOrganization deletes an organization (admin only).
func (r *AdminResource) DeleteOrganization(ctx context.Context, id string, hard bool) error {
	path := fmt.Sprintf("/api/v1/admin/organizations/%s", id)
	if hard {
		path += "?hard=true"
	}
	return r.base.AdminDelete(ctx, path)
}

// ResetUsage resets usage counters for an organization.
func (r *AdminResource) ResetUsage(ctx context.Context, orgID string) error {
	return r.base.AdminPost(ctx, fmt.Sprintf("/api/v1/admin/organizations/%s/reset-usage", orgID), nil, nil)
}

// CreateAPIKeyRequest is the admin request to create an API key for an org.
type AdminCreateAPIKeyRequest struct {
	Name      string     `json:"name"`
	Queues    []string   `json:"queues,omitempty"`
	RateLimit *int       `json:"rate_limit,omitempty"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

// CreateAPIKey creates an API key for an organization (admin only).
func (r *AdminResource) CreateAPIKey(ctx context.Context, orgID string, req *AdminCreateAPIKeyRequest) (*CreateAPIKeyResponse, error) {
	var result CreateAPIKeyResponse
	if err := r.base.AdminPost(ctx, fmt.Sprintf("/api/v1/admin/organizations/%s/api-keys", orgID), req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}


