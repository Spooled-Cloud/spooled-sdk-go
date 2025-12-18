package types

import "time"

// BillingStatus represents billing status information.
type BillingStatus struct {
	PlanTier                   PlanTier   `json:"plan_tier"`
	StripeSubscriptionID       *string    `json:"stripe_subscription_id,omitempty"`
	StripeSubscriptionStatus   *string    `json:"stripe_subscription_status,omitempty"`
	StripeCurrentPeriodEnd     *time.Time `json:"stripe_current_period_end,omitempty"`
	StripeCancelAtPeriodEnd    *bool      `json:"stripe_cancel_at_period_end,omitempty"`
	HasStripeCustomer          bool       `json:"has_stripe_customer"`
}

// CreatePortalRequest is the request to create a billing portal session.
type CreatePortalRequest struct {
	ReturnURL string `json:"return_url"`
}

// CreatePortalResponse is the response from creating a billing portal session.
type CreatePortalResponse struct {
	URL string `json:"url"`
}


