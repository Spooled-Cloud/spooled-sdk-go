package resources

import (
	"context"
	"time"

	"github.com/spooled-cloud/spooled-sdk-go/internal/httpx"
)

// BillingResource provides access to billing operations.
type BillingResource struct {
	base *Base
}

// NewBillingResource creates a new BillingResource.
func NewBillingResource(transport *httpx.Transport) *BillingResource {
	return &BillingResource{base: NewBase(transport)}
}

// BillingStatus represents billing status information.
type BillingStatus struct {
	PlanTier                 PlanTier   `json:"plan_tier"`
	StripeSubscriptionID     *string    `json:"stripe_subscription_id,omitempty"`
	StripeSubscriptionStatus *string    `json:"stripe_subscription_status,omitempty"`
	StripeCurrentPeriodEnd   *time.Time `json:"stripe_current_period_end,omitempty"`
	StripeCancelAtPeriodEnd  *bool      `json:"stripe_cancel_at_period_end,omitempty"`
	HasStripeCustomer        bool       `json:"has_stripe_customer"`
}

// GetStatus retrieves billing status for the current organization.
func (r *BillingResource) GetStatus(ctx context.Context) (*BillingStatus, error) {
	var result BillingStatus
	if err := r.base.Get(ctx, "/api/v1/billing/status", &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// CreatePortalRequest is the request to create a billing portal session.
type CreatePortalRequest struct {
	ReturnURL string `json:"return_url"`
}

// CreatePortalResponse is the response from creating a billing portal session.
type CreatePortalResponse struct {
	URL string `json:"url"`
}

// CreatePortalSession creates a Stripe billing portal session.
func (r *BillingResource) CreatePortalSession(ctx context.Context, req *CreatePortalRequest) (*CreatePortalResponse, error) {
	var result CreatePortalResponse
	if err := r.base.Post(ctx, "/api/v1/billing/portal", req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}
