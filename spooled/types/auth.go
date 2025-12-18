package types

import "time"

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

// ValidateResponse is the response from validating a token.
type ValidateResponse struct {
	Valid          bool       `json:"valid"`
	OrganizationID *string    `json:"organization_id,omitempty"`
	APIKeyID       *string    `json:"api_key_id,omitempty"`
	ExpiresAt      *time.Time `json:"expires_at,omitempty"`
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

// CheckEmailResponse is the response from checking if an email exists.
type CheckEmailResponse struct {
	Exists bool `json:"exists"`
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


