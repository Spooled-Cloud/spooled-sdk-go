package spooled

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
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

// parseErrorResponse parses an error response from the API.
func parseErrorResponse(resp *http.Response) error {
	baseErr := &APIError{
		StatusCode: resp.StatusCode,
		RequestID:  resp.Header.Get("X-Request-ID"),
	}

	// Read body
	if resp.Body != nil {
		body, err := io.ReadAll(resp.Body)
		if err == nil && len(body) > 0 {
			baseErr.RawBody = body
			// Try to parse JSON error response
			var apiErr struct {
				Code    string         `json:"code"`
				Message string         `json:"message"`
				Details map[string]any `json:"details"`
				Error   string         `json:"error"` // Some endpoints use "error" field
			}
			if json.Unmarshal(body, &apiErr) == nil {
				baseErr.Code = apiErr.Code
				baseErr.Message = apiErr.Message
				baseErr.Details = apiErr.Details
				if baseErr.Message == "" && apiErr.Error != "" {
					baseErr.Message = apiErr.Error
				}
			}
		}
	}

	// Create typed error based on status code
	switch resp.StatusCode {
	case http.StatusUnauthorized:
		return &AuthenticationError{APIError: baseErr}
	case http.StatusForbidden:
		return &AuthorizationError{APIError: baseErr}
	case http.StatusNotFound:
		return &NotFoundError{APIError: baseErr}
	case http.StatusConflict:
		return &ConflictError{APIError: baseErr}
	case http.StatusBadRequest, http.StatusUnprocessableEntity:
		return &ValidationError{APIError: baseErr}
	case http.StatusTooManyRequests:
		return parseRateLimitError(baseErr, resp)
	case http.StatusRequestEntityTooLarge:
		return &PayloadTooLargeError{APIError: baseErr}
	default:
		if resp.StatusCode >= 500 {
			return &ServerError{APIError: baseErr}
		}
		return baseErr
	}
}

// parseRateLimitError parses rate limit headers from the response.
func parseRateLimitError(baseErr *APIError, resp *http.Response) *RateLimitError {
	err := &RateLimitError{APIError: baseErr}

	// Parse Retry-After header
	if retryAfter := resp.Header.Get("Retry-After"); retryAfter != "" {
		if secs, parseErr := strconv.Atoi(retryAfter); parseErr == nil {
			err.RetryAfter = time.Duration(secs) * time.Second
		} else if t, parseErr := time.Parse(time.RFC1123, retryAfter); parseErr == nil {
			err.RetryAfter = time.Until(t)
		}
	}

	// Parse rate limit headers
	if limit := resp.Header.Get("X-RateLimit-Limit"); limit != "" {
		err.Limit, _ = strconv.Atoi(limit)
	}
	if remaining := resp.Header.Get("X-RateLimit-Remaining"); remaining != "" {
		err.Remaining, _ = strconv.Atoi(remaining)
	}
	if reset := resp.Header.Get("X-RateLimit-Reset"); reset != "" {
		if ts, parseErr := strconv.ParseInt(reset, 10, 64); parseErr == nil {
			err.Reset = time.Unix(ts, 0)
		}
	}

	return err
}

// newNetworkError creates a new network error.
func newNetworkError(err error) *NetworkError {
	return &NetworkError{
		APIError: &APIError{
			Code:    "network_error",
			Message: err.Error(),
			Err:     err,
		},
	}
}

// newTimeoutError creates a new timeout error.
func newTimeoutError(timeout time.Duration, err error) *TimeoutError {
	return &TimeoutError{
		APIError: &APIError{
			Code:    "timeout",
			Message: fmt.Sprintf("request timed out after %v", timeout),
			Err:     err,
		},
		TimeoutSeconds: timeout.Seconds(),
	}
}

// newCircuitBreakerOpenError creates a new circuit breaker open error.
func newCircuitBreakerOpenError() *CircuitBreakerOpenError {
	return &CircuitBreakerOpenError{
		APIError: &APIError{
			Code:    "circuit_breaker_open",
			Message: "circuit breaker is open",
		},
	}
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

// errorCodeFromStatus returns a default error code for an HTTP status.
func errorCodeFromStatus(status int) string {
	switch status {
	case http.StatusUnauthorized:
		return "unauthorized"
	case http.StatusForbidden:
		return "forbidden"
	case http.StatusNotFound:
		return "not_found"
	case http.StatusConflict:
		return "conflict"
	case http.StatusBadRequest:
		return "bad_request"
	case http.StatusUnprocessableEntity:
		return "validation_error"
	case http.StatusTooManyRequests:
		return "rate_limit_exceeded"
	case http.StatusRequestEntityTooLarge:
		return "payload_too_large"
	case http.StatusInternalServerError:
		return "internal_error"
	case http.StatusBadGateway:
		return "bad_gateway"
	case http.StatusServiceUnavailable:
		return "service_unavailable"
	case http.StatusGatewayTimeout:
		return "gateway_timeout"
	default:
		return "unknown_error"
	}
}

// Sentinel errors for common conditions
var (
	// ErrNoAuth is returned when no authentication is configured.
	ErrNoAuth = errors.New("no authentication configured: set APIKey or AccessToken")

	// ErrInvalidAPIKey is returned when the API key format is invalid.
	ErrInvalidAPIKey = errors.New("invalid API key format: must start with sp_live_ or sp_test_")

	// ErrCircuitOpen is returned when the circuit breaker is open.
	ErrCircuitOpen = errors.New("circuit breaker is open")
)

// ValidateAPIKey validates the format of an API key.
func ValidateAPIKey(key string) error {
	if key == "" {
		return ErrNoAuth
	}
	if !strings.HasPrefix(key, "sp_live_") && !strings.HasPrefix(key, "sp_test_") {
		return ErrInvalidAPIKey
	}
	if len(key) < 20 {
		return fmt.Errorf("API key is too short: expected at least 20 characters")
	}
	return nil
}
