package resources

import (
	"context"
	"encoding/json"
	"net/url"
	"time"

	"github.com/spooled-cloud/spooled-sdk-go/internal/httpx"
)

// AuthResource provides access to authentication operations.
type AuthResource struct {
	base *Base
}

// NewAuthResource creates a new AuthResource.
func NewAuthResource(transport *httpx.Transport) *AuthResource {
	return &AuthResource{base: NewBase(transport)}
}

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

// Login authenticates with an API key and returns JWT tokens.
func (r *AuthResource) Login(ctx context.Context, req *LoginRequest) (*LoginResponse, error) {
	var result LoginResponse
	if err := r.base.PostIdempotent(ctx, "/api/v1/auth/login", req, &result); err != nil {
		return nil, err
	}
	return &result, nil
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

// Refresh refreshes an access token using a refresh token.
func (r *AuthResource) Refresh(ctx context.Context, req *RefreshRequest) (*RefreshResponse, error) {
	var result RefreshResponse
	if err := r.base.PostIdempotent(ctx, "/api/v1/auth/refresh", req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// LogoutRequest is POST /auth/logout. refresh_token must be in the body or
// /auth/refresh still mints a new pair after the access token is blacklisted.
type LogoutRequest struct {
	RefreshToken string `json:"refresh_token,omitempty"`
}

// Logout invalidates the access token and, when available, the refresh token.
//
// POST /auth/logout blacklists the access token from Authorization. Without
// refresh_token in the body, /auth/refresh still mints a new pair. An explicit
// token is sent when given; otherwise the client's stored refresh token is used.
func (r *AuthResource) Logout(ctx context.Context, refreshToken ...string) error {
	token := ""
	if len(refreshToken) > 0 {
		token = refreshToken[0]
	}
	if token == "" {
		token = r.base.transport.GetRefreshToken()
	}
	var body any
	if token != "" {
		body = &LogoutRequest{RefreshToken: token}
	}
	return r.base.Post(ctx, "/api/v1/auth/logout", body, nil)
}

// MeResponse is the response from the /auth/me endpoint.
type MeResponse struct {
	OrganizationID string    `json:"organization_id"`
	APIKeyID       string    `json:"api_key_id"`
	Queues         []string  `json:"queues"`
	IssuedAt       time.Time `json:"issued_at"`
	ExpiresAt      time.Time `json:"expires_at"`
}

// Me retrieves information about the current authenticated session.
func (r *AuthResource) Me(ctx context.Context) (*MeResponse, error) {
	var result MeResponse
	if err := r.base.Get(ctx, "/api/v1/auth/me", &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// ValidateRequest is the request to validate a token.
type ValidateRequest struct {
	Token string `json:"token"`
}

// ValidateResponse is POST /auth/validate — `{ valid, error?, claims? }`.
//
// Claims carry org_id, api_key_id, queues, exp. The API never sends
// top-level organization_id / expires_at.
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

// Validate validates a token.
func (r *AuthResource) Validate(ctx context.Context, req *ValidateRequest) (*ValidateResponse, error) {
	var result ValidateResponse
	if err := r.base.PostIdempotent(ctx, "/api/v1/auth/validate", req, &result); err != nil {
		return nil, err
	}
	return &result, nil
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

// StartEmailLogin starts the email login flow by sending a login code.
func (r *AuthResource) StartEmailLogin(ctx context.Context, req *StartEmailLoginRequest) (*StartEmailLoginResponse, error) {
	var result StartEmailLoginResponse
	if err := r.base.Post(ctx, "/api/v1/auth/email/start", req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// CheckEmailResponse is GET /auth/check-email.
type CheckEmailResponse struct {
	Available     bool `json:"available"`
	Exists        bool `json:"exists"`
	SignupEnabled bool `json:"signup_enabled"`
}

// CheckEmail checks whether an email is registered (GET /auth/check-email).
func (r *AuthResource) CheckEmail(ctx context.Context, email string) (*CheckEmailResponse, error) {
	var result CheckEmailResponse
	query := url.Values{}
	query.Set("email", email)
	if err := r.base.GetWithQuery(ctx, "/api/v1/auth/check-email", query, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// VerifyEmailRequest is the request to verify an email login code.
type VerifyEmailRequest struct {
	Email string `json:"email"`
	Code  string `json:"code"`
}

// VerifyEmailResponse is POST /auth/email/verify — tagged `{type: login|signup, ...}`.
//
// Login sends access/refresh tokens. Signup (no account yet) sends
// signup_token and never access_token.
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

// VerifyEmail verifies an email login code.
//
// Existing accounts return type "login" with tokens. New emails return type
// "signup" with signup_token; mapping that onto token fields dropped the
// token needed for POST /auth/signup/complete.
func (r *AuthResource) VerifyEmail(ctx context.Context, req *VerifyEmailRequest) (*VerifyEmailResponse, error) {
	var result VerifyEmailResponse
	if err := r.base.Post(ctx, "/api/v1/auth/email/verify", req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}
