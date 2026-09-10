package types

import (
	"encoding/json"
	"time"
)

// LoginRequest is the request to login with an API key.
type LoginRequest struct {
	APIKey string `json:"api_key"`
}

// LoginResponse is the response from logging in.
type LoginResponse struct {
	AccessToken      string `json:"access_token"`
	RefreshToken     string `json:"refresh_token"`
	TokenType        string `json:"token_type"`
	ExpiresIn        int    `json:"expires_in"`
	RefreshExpiresIn int    `json:"refresh_expires_in"`
}

// RefreshRequest is the request to refresh a token.
type RefreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

// RefreshResponse is the response from refreshing a token.
type RefreshResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
}

// MeResponse is the response from the /auth/me endpoint.
type MeResponse struct {
	OrganizationID string    `json:"organization_id"`
	APIKeyID       string    `json:"api_key_id"`
	Queues         []string  `json:"queues"`
	IssuedAt       time.Time `json:"issued_at"`
	ExpiresAt      time.Time `json:"expires_at"`
}

// ValidateRequest is the request to validate a token.
type ValidateRequest struct {
	Token string `json:"token"`
}

// ValidateResponse is POST /auth/validate — `{ valid, error?, claims? }`.
type ValidateResponse struct {
	Valid          bool       `json:"valid"`
	Error          *string    `json:"error,omitempty"`
	OrganizationID *string    `json:"organization_id,omitempty"`
	APIKeyID       *string    `json:"api_key_id,omitempty"`
	Queues         []string   `json:"queues,omitempty"`
	ExpiresAt      *time.Time `json:"expires_at,omitempty"`
}

// UnmarshalJSON maps claims.org_id / api_key_id / queues / exp onto the
// exported fields. A valid token otherwise unmarshals as empty IDs.
func (v *ValidateResponse) UnmarshalJSON(data []byte) error {
	type alias ValidateResponse
	aux := struct {
		*alias
		Claims *struct {
			OrgID    *string  `json:"org_id"`
			APIKeyID *string  `json:"api_key_id"`
			Queues   []string `json:"queues"`
			Exp      *int64   `json:"exp"`
		} `json:"claims"`
	}{alias: (*alias)(v)}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	if aux.Claims == nil {
		return nil
	}
	if v.OrganizationID == nil && aux.Claims.OrgID != nil {
		v.OrganizationID = aux.Claims.OrgID
	}
	if v.APIKeyID == nil && aux.Claims.APIKeyID != nil {
		v.APIKeyID = aux.Claims.APIKeyID
	}
	if len(v.Queues) == 0 && aux.Claims.Queues != nil {
		v.Queues = aux.Claims.Queues
	}
	if v.ExpiresAt == nil && aux.Claims.Exp != nil {
		t := time.Unix(*aux.Claims.Exp, 0).UTC()
		v.ExpiresAt = &t
	}
	return nil
}

// StartEmailLoginRequest is the request to start email login.
type StartEmailLoginRequest struct {
	Email string `json:"email"`
}

// StartEmailLoginResponse is POST /auth/email/start.
type StartEmailLoginResponse struct {
	Message     string `json:"message"`
	EmailSentTo string `json:"email_sent_to"`
}

// CheckEmailResponse is GET /auth/check-email.
type CheckEmailResponse struct {
	Available     bool `json:"available"`
	Exists        bool `json:"exists"`
	SignupEnabled bool `json:"signup_enabled"`
}

// VerifyEmailRequest is the request to verify an email login code.
type VerifyEmailRequest struct {
	Email string `json:"email"`
	Code  string `json:"code"`
}

// VerifyEmailResponse is POST /auth/email/verify — tagged `{type: login|signup, ...}`.
type VerifyEmailResponse struct {
	Type             string `json:"type"`
	AccessToken      string `json:"access_token,omitempty"`
	RefreshToken     string `json:"refresh_token,omitempty"`
	TokenType        string `json:"token_type,omitempty"`
	ExpiresIn        int    `json:"expires_in,omitempty"`
	RefreshExpiresIn int    `json:"refresh_expires_in,omitempty"`
	SignupToken      string `json:"signup_token,omitempty"`
	Email            string `json:"email,omitempty"`
}
