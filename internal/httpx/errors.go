package httpx

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// APIError is the base error type for all HTTP errors.
type APIError struct {
	StatusCode int            `json:"status_code,omitempty"`
	Code       string         `json:"code,omitempty"`
	Message    string         `json:"message,omitempty"`
	Details    map[string]any `json:"details,omitempty"`
	RequestID  string         `json:"request_id,omitempty"`
	RawBody    []byte         `json:"-"`
	Err        error          `json:"-"`
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

// IsRetryable returns true if the error is retryable: 5xx, 408 and 429. Every
// other 4xx is a deterministic rejection and retrying it cannot succeed.
func (e *APIError) IsRetryable() bool {
	if e.StatusCode >= 500 && e.StatusCode < 600 {
		return true
	}
	return e.StatusCode == http.StatusTooManyRequests || e.StatusCode == http.StatusRequestTimeout
}

// AuthenticationError represents a 401 error.
type AuthenticationError struct{ *APIError }

// Unwrap returns the underlying API error.
func (e *AuthenticationError) Unwrap() error { return e.APIError }

// AuthorizationError represents a 403 error.
type AuthorizationError struct{ *APIError }

// Unwrap returns the underlying API error.
func (e *AuthorizationError) Unwrap() error { return e.APIError }

// NotFoundError represents a 404 error.
type NotFoundError struct{ *APIError }

// Unwrap returns the underlying API error.
func (e *NotFoundError) Unwrap() error { return e.APIError }

// ConflictError represents a 409 error.
type ConflictError struct{ *APIError }

// Unwrap returns the underlying API error.
func (e *ConflictError) Unwrap() error { return e.APIError }

// ValidationError represents a 400/422 error.
type ValidationError struct{ *APIError }

// Unwrap returns the underlying API error.
func (e *ValidationError) Unwrap() error { return e.APIError }

// QuotaInfo describes the plan limit behind a 429 QUOTA_EXCEEDED response.
type QuotaInfo struct {
	// Resource is the limited resource, e.g. "active_jobs" or "queues".
	Resource string
	// Current is the usage that tripped the limit.
	Current int64
	// Limit is the plan limit for Resource.
	Limit int64
	// Plan is the organization's plan tier.
	Plan string
	// UpgradeTo is the plan that lifts the limit, when the API names one.
	UpgradeTo string
}

// RateLimitError represents a 429 error.
//
// A 429 comes in two kinds. A rate limit (RATE_LIMIT_EXCEEDED, usually with a
// Retry-After header) clears by itself and is retried. A plan quota
// (QUOTA_EXCEEDED, Quota != nil) only clears when the customer frees capacity or
// upgrades, so it is never retried and never counts against the circuit breaker.
type RateLimitError struct {
	*APIError
	RetryAfter time.Duration
	Limit      int
	Remaining  int
	Reset      time.Time
	// Quota is set when the 429 is a plan-limit rejection rather than a rate limit.
	Quota *QuotaInfo
}

// Unwrap returns the underlying API error.
func (e *RateLimitError) Unwrap() error { return e.APIError }

// IsQuotaExceeded reports whether this 429 is a plan-limit rejection
// (QUOTA_EXCEEDED) instead of a transient rate limit.
func (e *RateLimitError) IsQuotaExceeded() bool { return e.Quota != nil }

// IsRetryable is false for a plan quota: waiting does not free capacity, so
// retrying only burns long sleeps and trips the circuit breaker for traffic
// (worker acks, heartbeats) that is unrelated to the over-quota enqueue.
func (e *RateLimitError) IsRetryable() bool { return e.Quota == nil }

// GetRetryAfter returns the retry-after duration in seconds.
func (e *RateLimitError) GetRetryAfter() int {
	return int(e.RetryAfter.Seconds())
}

// PayloadTooLargeError represents a 413 error.
type PayloadTooLargeError struct{ *APIError }

// Unwrap returns the underlying API error.
func (e *PayloadTooLargeError) Unwrap() error { return e.APIError }

// ServerError represents a 5xx error.
type ServerError struct{ *APIError }

// Unwrap returns the underlying API error.
func (e *ServerError) Unwrap() error { return e.APIError }

// IsRetryable always returns true for server errors.
func (e *ServerError) IsRetryable() bool { return true }

// NetworkError represents a network-level error.
type NetworkError struct{ *APIError }

// Unwrap returns the underlying API error.
func (e *NetworkError) Unwrap() error { return e.APIError }

// IsRetryable always returns true for network errors.
func (e *NetworkError) IsRetryable() bool { return true }

// TimeoutError represents a timeout error.
type TimeoutError struct {
	*APIError
	TimeoutSeconds float64
}

// Unwrap returns the underlying API error.
func (e *TimeoutError) Unwrap() error { return e.APIError }

// IsRetryable always returns true for timeout errors.
func (e *TimeoutError) IsRetryable() bool { return true }

// CircuitBreakerOpenError represents a circuit breaker open error.
type CircuitBreakerOpenError struct{ *APIError }

// Unwrap returns the underlying API error.
func (e *CircuitBreakerOpenError) Unwrap() error { return e.APIError }

// IsRetryable always returns false for circuit breaker errors.
func (e *CircuitBreakerOpenError) IsRetryable() bool { return false }

// ParseErrorFromResponse parses an error from an HTTP response.
func ParseErrorFromResponse(statusCode int, body []byte, headers http.Header) error {
	baseErr := &APIError{
		StatusCode: statusCode,
		RequestID:  headers.Get("X-Request-ID"),
		RawBody:    body,
	}

	// Parse JSON body.
	//
	// `details` is decoded as a raw message rather than a map[string]any because
	// the server sends it in two shapes: an object (a map of field → info) for
	// most errors, and an ARRAY of per-field errors for validation failures
	// (e.g. {"code":"VALIDATION_ERROR","details":[{"field":...}]}). Decoding the
	// array straight into a map used to fail the whole json.Unmarshal, silently
	// dropping Code/Message/Details. Deferring the details shape to parseDetails
	// keeps Code and Message populated regardless of which shape arrives.
	if len(body) > 0 {
		var apiErr struct {
			Code    string          `json:"code"`
			Message string          `json:"message"`
			Details json.RawMessage `json:"details"`
			Error   string          `json:"error"`
		}
		if json.Unmarshal(body, &apiErr) == nil {
			baseErr.Code = apiErr.Code
			baseErr.Message = apiErr.Message
			baseErr.Details = parseDetails(apiErr.Details)
			if baseErr.Message == "" && apiErr.Error != "" {
				baseErr.Message = apiErr.Error
			}
		}
	}

	switch statusCode {
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
		return parseRateLimitError(baseErr, headers)
	case http.StatusRequestEntityTooLarge:
		return &PayloadTooLargeError{APIError: baseErr}
	default:
		if statusCode >= 500 {
			return &ServerError{APIError: baseErr}
		}
		return baseErr
	}
}

// parseDetails normalizes the `details` field of an error body into a
// map[string]any so callers always read APIError.Details the same way.
//
// The server sends `details` as either a JSON object (used as-is) or a JSON
// array of per-field validation errors. An array is wrapped under the "errors"
// key so it remains accessible as err.Details["errors"]. A null, absent, or
// otherwise non-object/array value yields nil.
func parseDetails(raw json.RawMessage) map[string]any {
	if len(raw) == 0 {
		return nil
	}
	// Object shape (or JSON null, which decodes to a nil map without error).
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err == nil {
		return obj
	}
	// Array shape: wrap so the field errors stay reachable via a stable key.
	var arr []any
	if err := json.Unmarshal(raw, &arr); err == nil {
		return map[string]any{"errors": arr}
	}
	return nil
}

func parseRateLimitError(baseErr *APIError, headers http.Header) *RateLimitError {
	err := &RateLimitError{APIError: baseErr}
	err.Quota = parseQuotaInfo(baseErr, headers)

	if retryAfter := headers.Get("Retry-After"); retryAfter != "" {
		if secs, parseErr := strconv.Atoi(retryAfter); parseErr == nil {
			err.RetryAfter = time.Duration(secs) * time.Second
		} else if t, parseErr := time.Parse(time.RFC1123, retryAfter); parseErr == nil {
			err.RetryAfter = time.Until(t)
		}
	}

	// Try both canonical and non-canonical header names
	if limit := headers.Get("X-Ratelimit-Limit"); limit != "" {
		err.Limit, _ = strconv.Atoi(limit)
	}
	if remaining := headers.Get("X-Ratelimit-Remaining"); remaining != "" {
		err.Remaining, _ = strconv.Atoi(remaining)
	}
	if reset := headers.Get("X-Ratelimit-Reset"); reset != "" {
		if ts, parseErr := strconv.ParseInt(reset, 10, 64); parseErr == nil {
			err.Reset = time.Unix(ts, 0)
		}
	}

	return err
}

// parseQuotaInfo returns the plan-limit details when a 429 is a quota rejection.
//
// The API answers an over-quota request with 429 and
// {"error":"limit_exceeded","code":"QUOTA_EXCEEDED","resource":...,"current":...,
// "limit":...,"plan":...}. It carries no Retry-After header, unlike a real rate
// limit. A body with that plan-limit shape and no Retry-After is treated as a
// quota even if an older server omitted the code.
func parseQuotaInfo(baseErr *APIError, headers http.Header) *QuotaInfo {
	var body struct {
		Error     string `json:"error"`
		Resource  string `json:"resource"`
		Current   int64  `json:"current"`
		Limit     *int64 `json:"limit"`
		Plan      string `json:"plan"`
		UpgradeTo string `json:"upgrade_to"`
	}
	if len(baseErr.RawBody) > 0 {
		_ = json.Unmarshal(baseErr.RawBody, &body)
	}

	isQuota := strings.EqualFold(baseErr.Code, "QUOTA_EXCEEDED") || body.Error == "limit_exceeded"
	if !isQuota && headers.Get("Retry-After") == "" && body.Resource != "" && body.Plan != "" && body.Limit != nil {
		isQuota = true
	}
	if !isQuota {
		return nil
	}

	info := &QuotaInfo{
		Resource:  body.Resource,
		Current:   body.Current,
		Plan:      body.Plan,
		UpgradeTo: body.UpgradeTo,
	}
	if body.Limit != nil {
		info.Limit = *body.Limit
	}
	return info
}

// NewNetworkError creates a new network error.
func NewNetworkError(err error) *NetworkError {
	return &NetworkError{
		APIError: &APIError{
			Code:    "network_error",
			Message: err.Error(),
			Err:     err,
		},
	}
}

// NewTimeoutError creates a new timeout error.
func NewTimeoutError(timeout time.Duration, err error) *TimeoutError {
	return &TimeoutError{
		APIError: &APIError{
			Code:    "timeout",
			Message: fmt.Sprintf("request timed out after %v", timeout),
			Err:     err,
		},
		TimeoutSeconds: timeout.Seconds(),
	}
}

// NewCircuitBreakerOpenError creates a new circuit breaker open error.
func NewCircuitBreakerOpenError() *CircuitBreakerOpenError {
	return &CircuitBreakerOpenError{
		APIError: &APIError{
			Code:    "circuit_breaker_open",
			Message: "circuit breaker is open",
		},
	}
}

// IsRetryable returns true if the error is retryable.
func IsRetryable(err error) bool {
	if err == nil {
		return false
	}

	// Check for retryable interface
	type retryable interface {
		IsRetryable() bool
	}
	if r, ok := err.(retryable); ok {
		return r.IsRetryable()
	}

	// Check for base APIError
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.IsRetryable()
	}

	return false
}

// IsAuthenticationError returns true if the error is a 401 error.
func IsAuthenticationError(err error) bool {
	var authErr *AuthenticationError
	return errors.As(err, &authErr)
}

// IsNotFoundError returns true if the error is a 404 error.
func IsNotFoundError(err error) bool {
	var notFoundErr *NotFoundError
	return errors.As(err, &notFoundErr)
}

// IsRateLimitError returns true if the error is a 429 error.
func IsRateLimitError(err error) bool {
	var rateLimitErr *RateLimitError
	return errors.As(err, &rateLimitErr)
}

// IsValidationError returns true if the error is a 400/422 error.
func IsValidationError(err error) bool {
	var validationErr *ValidationError
	return errors.As(err, &validationErr)
}

// AsAPIError extracts the underlying API error.
func AsAPIError(err error) (*APIError, bool) {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr, true
	}
	return nil, false
}
