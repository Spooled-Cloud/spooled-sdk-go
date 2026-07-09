package spooled

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spooled-cloud/spooled-sdk-go/internal/httpx"
)

// The public error types below are aliases for the concrete error types that
// the transport actually returns. Because they are type aliases (not distinct
// wrapper types), errors.As / errors.Is and every Is*Error helper in this file
// match the values the client hands back from real API calls.
//
// Historically this package declared a parallel set of struct types that were
// never produced anywhere, so the helpers (and the documented
// errors.As(err, &spooled.RateLimitError{}) pattern) silently never matched.
// Aliasing collapses the two hierarchies into one, which is the fix.

// APIError is the base error type for all Spooled SDK errors.
//
// Every typed error below embeds *APIError, so errors.As(err, &apiErr) with an
// *APIError target succeeds for any Spooled API error. Use AsSpooledError for a
// convenient accessor.
type APIError = httpx.APIError

// AuthenticationError represents a 401 error.
type AuthenticationError = httpx.AuthenticationError

// AuthorizationError represents a 403 error.
type AuthorizationError = httpx.AuthorizationError

// NotFoundError represents a 404 error.
type NotFoundError = httpx.NotFoundError

// ConflictError represents a 409 error.
type ConflictError = httpx.ConflictError

// ValidationError represents a 400/422 error.
type ValidationError = httpx.ValidationError

// RateLimitError represents a 429 error. It carries rate-limit metadata parsed
// from the response headers (RetryAfter, Limit, Remaining, Reset).
type RateLimitError = httpx.RateLimitError

// PayloadTooLargeError represents a 413 error.
type PayloadTooLargeError = httpx.PayloadTooLargeError

// ServerError represents a 5xx error. It is always retryable.
type ServerError = httpx.ServerError

// NetworkError represents a network-level error. It is always retryable.
type NetworkError = httpx.NetworkError

// TimeoutError represents a timeout error. It is always retryable.
type TimeoutError = httpx.TimeoutError

// CircuitBreakerOpenError is returned when the circuit breaker is open.
type CircuitBreakerOpenError = httpx.CircuitBreakerOpenError

// IsSpooledError returns true if the error is (or wraps) a Spooled SDK API error.
func IsSpooledError(err error) bool {
	var spErr *APIError
	return errors.As(err, &spErr)
}

// AsSpooledError attempts to extract the underlying Spooled APIError from err.
// It returns the *APIError and true if err is (or wraps) a Spooled API error.
func AsSpooledError(err error) (*APIError, bool) {
	var spErr *APIError
	if errors.As(err, &spErr) {
		return spErr, true
	}
	return nil, false
}

// IsRetryable returns true if the error is retryable (5xx, 429, network, or
// timeout errors).
func IsRetryable(err error) bool {
	if err == nil {
		return false
	}

	// Check for typed error interfaces first (ServerError, NetworkError,
	// TimeoutError, and CircuitBreakerOpenError override IsRetryable).
	type retryable interface {
		IsRetryable() bool
	}
	if r, ok := err.(retryable); ok {
		return r.IsRetryable()
	}

	// Fall back to the base APIError, which classifies by status code.
	var spErr *APIError
	if errors.As(err, &spErr) {
		return spErr.IsRetryable()
	}

	return false
}

// IsAuthenticationError returns true if the error is a 401 authentication error.
func IsAuthenticationError(err error) bool {
	var authErr *AuthenticationError
	return errors.As(err, &authErr)
}

// IsAuthorizationError returns true if the error is a 403 authorization error.
func IsAuthorizationError(err error) bool {
	var authzErr *AuthorizationError
	return errors.As(err, &authzErr)
}

// IsNotFoundError returns true if the error is a 404 not-found error.
func IsNotFoundError(err error) bool {
	var notFoundErr *NotFoundError
	return errors.As(err, &notFoundErr)
}

// IsConflictError returns true if the error is a 409 conflict error.
func IsConflictError(err error) bool {
	var conflictErr *ConflictError
	return errors.As(err, &conflictErr)
}

// IsRateLimitError returns true if the error is a 429 rate-limit error.
func IsRateLimitError(err error) bool {
	var rateLimitErr *RateLimitError
	return errors.As(err, &rateLimitErr)
}

// IsValidationError returns true if the error is a 400/422 validation error.
func IsValidationError(err error) bool {
	var validationErr *ValidationError
	return errors.As(err, &validationErr)
}

// IsPayloadTooLargeError returns true if the error is a 413 payload-too-large error.
func IsPayloadTooLargeError(err error) bool {
	var payloadErr *PayloadTooLargeError
	return errors.As(err, &payloadErr)
}

// IsServerError returns true if the error is a 5xx server error.
func IsServerError(err error) bool {
	var serverErr *ServerError
	return errors.As(err, &serverErr)
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
//
// The key is trimmed of surrounding whitespace (including trailing newlines)
// before validation so callers reading keys from files or environment
// variables do not need to normalize input themselves. This mirrors the
// trimming performed by `resolveConfig` so `ValidateAPIKey` and the
// eventual `Authorization` header agree on what a "valid" key is.
func ValidateAPIKey(key string) error {
	key = strings.TrimSpace(key)
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
