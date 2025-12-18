package resources

import (
	"context"
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

// Logout invalidates the current tokens.
func (r *AuthResource) Logout(ctx context.Context) error {
	return r.base.Post(ctx, "/api/v1/auth/logout", nil, nil)
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

// ValidateResponse is the response from validating a token.
type ValidateResponse struct {
	Valid          bool       `json:"valid"`
	OrganizationID *string    `json:"organization_id,omitempty"`
	APIKeyID       *string    `json:"api_key_id,omitempty"`
	ExpiresAt      *time.Time `json:"expires_at,omitempty"`
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

// StartEmailLoginResponse is the response from starting email login.
type StartEmailLoginResponse struct {
	Success bool    `json:"success"`
	Message *string `json:"message,omitempty"`
}

// StartEmailLogin starts the email login flow by sending a login code.
func (r *AuthResource) StartEmailLogin(ctx context.Context, req *StartEmailLoginRequest) (*StartEmailLoginResponse, error) {
	var result StartEmailLoginResponse
	if err := r.base.Post(ctx, "/api/v1/auth/email/start", req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// CheckEmailResponse is the response from checking if an email exists.
type CheckEmailResponse struct {
	Exists bool `json:"exists"`
}

// CheckEmail checks if an email exists in the system.
func (r *AuthResource) CheckEmail(ctx context.Context, email string) (*CheckEmailResponse, error) {
	var result CheckEmailResponse
	if err := r.base.Post(ctx, "/api/v1/auth/email/check", map[string]string{"email": email}, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// VerifyEmailRequest is the request to verify an email login code.
type VerifyEmailRequest struct {
	Email string `json:"email"`
	Code  string `json:"code"`
}

// VerifyEmailResponse is the response from verifying an email login.
type VerifyEmailResponse struct {
	AccessToken      string `json:"access_token"`
	RefreshToken     string `json:"refresh_token"`
	TokenType        string `json:"token_type"`
	ExpiresIn        int    `json:"expires_in"`
	RefreshExpiresIn int    `json:"refresh_expires_in"`
}

// VerifyEmail verifies an email login code and returns tokens.
func (r *AuthResource) VerifyEmail(ctx context.Context, req *VerifyEmailRequest) (*VerifyEmailResponse, error) {
	var result VerifyEmailResponse
	if err := r.base.Post(ctx, "/api/v1/auth/email/verify", req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}
