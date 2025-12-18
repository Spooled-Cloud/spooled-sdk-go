package spooled

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// APIError is the base error type for all Spooled SDK errors.
type APIError struct {
	// StatusCode is the HTTP status code.
	StatusCode int `json:"status_code,omitempty"`
	// Code is the error code from the API.
	Code string `json:"code,omitempty"`
	// Message is the human-readable error message.
	Message string `json:"message,omitempty"`
	// Details contains additional error details.
	Details map[string]any `json:"details,omitempty"`
	// RequestID is the request ID for debugging.
	RequestID string `json:"request_id,omitempty"`
	// RawBody is the raw response body.
	RawBody []byte `json:"-"`
	// Err is the underlying error, if any.
	Err error `json:"-"`
}

// Error implements the error interface.
func (e *APIError) Error() string {
	if e.Message != "" {
		if e.Code != "" {
			return fmt.Sprintf("[%d] %s: %s", e.StatusCode, e.Code, e.Message)
		}
		return fmt.Sprintf("[%d] %s", e.StatusCode, e.Message)
	}
	if e.Code != "" {
		return fmt.Sprintf("[%d] %s", e.StatusCode, e.Code)
	}
	return fmt.Sprintf("[%d] unknown error", e.StatusCode)
}

// Unwrap returns the underlying error.
func (e *APIError) Unwrap() error {
	return e.Err
}

// IsRetryable returns true if the error is retryable.
func (e *APIError) IsRetryable() bool {
	// Network errors, timeouts, 5xx, and 429 are retryable
	if e.StatusCode >= 500 && e.StatusCode < 600 {
		return true
	}
	if e.StatusCode == http.StatusTooManyRequests {
		return true
	}
	return false
}

// AuthenticationError represents a 401 error.
type AuthenticationError struct {
	*APIError
}

// AuthorizationError represents a 403 error.
type AuthorizationError struct {
	*APIError
}

// NotFoundError represents a 404 error.
type NotFoundError struct {
	*APIError
}

// ConflictError represents a 409 error.
type ConflictError struct {
	*APIError
}

// ValidationError represents a 400/422 error.
type ValidationError struct {
	*APIError
}

// RateLimitError represents a 429 error.
type RateLimitError struct {
	*APIError
	// RetryAfter is the duration to wait before retrying.
	RetryAfter time.Duration
	// Limit is the rate limit.
	Limit int
	// Remaining is the remaining requests.
	Remaining int
	// Reset is the time when the rate limit resets.
	Reset time.Time
}

// GetRetryAfter returns the retry-after duration in seconds.
func (e *RateLimitError) GetRetryAfter() int {
	return int(e.RetryAfter.Seconds())
}

// PayloadTooLargeError represents a 413 error.
type PayloadTooLargeError struct {
	*APIError
}

// ServerError represents a 5xx error.
type ServerError struct {
	*APIError
}

// IsRetryable always returns true for server errors.
func (e *ServerError) IsRetryable() bool {
	return true
}

// NetworkError represents a network-level error.
type NetworkError struct {
	*APIError
}

// IsRetryable always returns true for network errors.
func (e *NetworkError) IsRetryable() bool {
	return true
}

// TimeoutError represents a timeout error.
type TimeoutError struct {
	*APIError
	// TimeoutSeconds is the timeout duration in seconds.
	TimeoutSeconds float64
}

// IsRetryable always returns true for timeout errors.
func (e *TimeoutError) IsRetryable() bool {
	return true
}

// CircuitBreakerOpenError represents a circuit breaker open error.
type CircuitBreakerOpenError struct {
	*APIError
}

// IsRetryable always returns false for circuit breaker errors.
func (e *CircuitBreakerOpenError) IsRetryable() bool {
	return false
}

// IsSpooledError returns true if the error is a Spooled SDK error.
func IsSpooledError(err error) bool {
	var spErr *APIError
	return errors.As(err, &spErr)
}

// AsSpooledError attempts to convert an error to a Spooled APIError.
func AsSpooledError(err error) (*APIError, bool) {
	var spErr *APIError
	if errors.As(err, &spErr) {
		return spErr, true
	}
	return nil, false
}

// IsRetryable returns true if the error is retryable.
func IsRetryable(err error) bool {
	// Check for typed error interfaces
	type retryable interface {
		IsRetryable() bool
	}
	if r, ok := err.(retryable); ok {
		return r.IsRetryable()
	}

	// Check for base APIError
	var spErr *APIError
	if errors.As(err, &spErr) {
		return spErr.IsRetryable()
	}

	return false
}

// IsAuthenticationError returns true if the error is an authentication error.
func IsAuthenticationError(err error) bool {
	var authErr *AuthenticationError
	return errors.As(err, &authErr)
}

// IsNotFoundError returns true if the error is a not found error.
func IsNotFoundError(err error) bool {
	var notFoundErr *NotFoundError
	return errors.As(err, &notFoundErr)
}

// IsRateLimitError returns true if the error is a rate limit error.
func IsRateLimitError(err error) bool {
	var rateLimitErr *RateLimitError
	return errors.As(err, &rateLimitErr)
}

// IsValidationError returns true if the error is a validation error.
func IsValidationError(err error) bool {
	var validationErr *ValidationError
	return errors.As(err, &validationErr)
}

// Sentinel errors for common conditions
var (
	// ErrNoAuth is returned when no authentication is configured.
	ErrNoAuth = errors.New("no authentication configured: set APIKey or AccessToken")

	// ErrInvalidAPIKey is returned when the API key format is invalid.
	ErrInvalidAPIKey = errors.New("invalid API key format: must start with sk_live_, sk_test_, sp_live_, or sp_test_")

	// ErrCircuitOpen is returned when the circuit breaker is open.
	ErrCircuitOpen = errors.New("circuit breaker is open")
)

// ValidateAPIKey validates the format of an API key.
// Accepts both sk_ (production keys) and sp_ (documentation examples) prefixes.
func ValidateAPIKey(key string) error {
	if key == "" {
		return ErrNoAuth
	}
	// Accept both sk_ (production keys) and sp_ (doc examples to avoid GitHub secret scanning)
	validPrefixes := []string{"sk_live_", "sk_test_", "sp_live_", "sp_test_"}
	hasValidPrefix := false
	for _, prefix := range validPrefixes {
		if strings.HasPrefix(key, prefix) {
			hasValidPrefix = true
			break
		}
	}
	if !hasValidPrefix {
		return ErrInvalidAPIKey
	}
	if len(key) < 20 {
		return fmt.Errorf("API key is too short: expected at least 20 characters")
	}
	return nil
}
