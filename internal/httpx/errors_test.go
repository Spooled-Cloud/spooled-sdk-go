package httpx

import (
	"errors"
	"net/http"
	"testing"
	"time"
)

func TestParseErrorFromResponse(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		body       []byte
		headers    http.Header
		checkType  func(error) bool
		checkMsg   string
	}{
		{
			name:       "401 authentication error",
			statusCode: 401,
			body:       []byte(`{"code":"invalid_token","message":"Token expired"}`),
			headers:    http.Header{"X-Request-ID": []string{"req-123"}},
			checkType:  func(err error) bool { return IsAuthenticationError(err) },
			checkMsg:   "[401] invalid_token: Token expired",
		},
		{
			name:       "403 authorization error",
			statusCode: 403,
			body:       []byte(`{"code":"forbidden","message":"Access denied"}`),
			headers:    http.Header{},
			checkType: func(err error) bool {
				var authErr *AuthorizationError
				return errors.As(err, &authErr)
			},
			checkMsg: "[403] forbidden: Access denied",
		},
		{
			name:       "404 not found error",
			statusCode: 404,
			body:       []byte(`{"code":"not_found","message":"Job not found"}`),
			headers:    http.Header{},
			checkType:  func(err error) bool { return IsNotFoundError(err) },
			checkMsg:   "[404] not_found: Job not found",
		},
		{
			name:       "409 conflict error",
			statusCode: 409,
			body:       []byte(`{"code":"conflict","message":"Resource already exists"}`),
			headers:    http.Header{},
			checkType: func(err error) bool {
				var conflictErr *ConflictError
				return errors.As(err, &conflictErr)
			},
			checkMsg: "[409] conflict: Resource already exists",
		},
		{
			name:       "400 validation error",
			statusCode: 400,
			body:       []byte(`{"code":"validation_error","message":"Invalid payload"}`),
			headers:    http.Header{},
			checkType:  func(err error) bool { return IsValidationError(err) },
			checkMsg:   "[400] validation_error: Invalid payload",
		},
		{
			name:       "422 validation error",
			statusCode: 422,
			body:       []byte(`{"code":"validation_error","message":"Unprocessable entity"}`),
			headers:    http.Header{},
			checkType:  func(err error) bool { return IsValidationError(err) },
			checkMsg:   "[422] validation_error: Unprocessable entity",
		},
		{
			name:       "413 payload too large error",
			statusCode: 413,
			body:       []byte(`{"code":"payload_too_large","message":"Request body too large"}`),
			headers:    http.Header{},
			checkType: func(err error) bool {
				var payloadErr *PayloadTooLargeError
				return errors.As(err, &payloadErr)
			},
			checkMsg: "[413] payload_too_large: Request body too large",
		},
		{
			name:       "500 server error",
			statusCode: 500,
			body:       []byte(`{"code":"internal_error","message":"Internal server error"}`),
			headers:    http.Header{},
			checkType: func(err error) bool {
				var serverErr *ServerError
				return errors.As(err, &serverErr)
			},
			checkMsg: "[500] internal_error: Internal server error",
		},
		{
			name:       "502 server error",
			statusCode: 502,
			body:       []byte(`{"message":"Bad gateway"}`),
			headers:    http.Header{},
			checkType: func(err error) bool {
				var serverErr *ServerError
				return errors.As(err, &serverErr)
			},
			checkMsg: "[502] Bad gateway",
		},
		{
			name:       "error with 'error' field instead of 'message'",
			statusCode: 400,
			body:       []byte(`{"error":"Invalid request"}`),
			headers:    http.Header{},
			checkType:  func(err error) bool { return IsValidationError(err) },
			checkMsg:   "[400] Invalid request",
		},
		{
			name:       "empty body",
			statusCode: 500,
			body:       nil,
			headers:    http.Header{},
			checkType: func(err error) bool {
				var serverErr *ServerError
				return errors.As(err, &serverErr)
			},
			checkMsg: "[500] unknown error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ParseErrorFromResponse(tt.statusCode, tt.body, tt.headers)

			if !tt.checkType(err) {
				t.Errorf("Expected error type check to pass for %v", err)
			}

			if err.Error() != tt.checkMsg {
				t.Errorf("Error() = %q, want %q", err.Error(), tt.checkMsg)
			}
		})
	}
}

func TestParseErrorFromResponse_RateLimit(t *testing.T) {
	headers := http.Header{}
	headers.Set("Retry-After", "60")
	headers.Set("X-Ratelimit-Limit", "100")
	headers.Set("X-Ratelimit-Remaining", "0")
	headers.Set("X-Ratelimit-Reset", "1700000000")
	headers.Set("X-Request-Id", "req-456")

	err := ParseErrorFromResponse(429, []byte(`{"code":"rate_limit_exceeded","message":"Too many requests"}`), headers)

	if !IsRateLimitError(err) {
		t.Fatal("Expected rate limit error")
	}

	var rateLimitErr *RateLimitError
	if !errors.As(err, &rateLimitErr) {
		t.Fatal("Expected to extract RateLimitError")
	}

	if rateLimitErr.RetryAfter != 60*time.Second {
		t.Errorf("RetryAfter = %v, want 60s", rateLimitErr.RetryAfter)
	}

	if rateLimitErr.GetRetryAfter() != 60 {
		t.Errorf("GetRetryAfter() = %d, want 60", rateLimitErr.GetRetryAfter())
	}

	if rateLimitErr.Limit != 100 {
		t.Errorf("Limit = %d, want 100", rateLimitErr.Limit)
	}

	if rateLimitErr.Remaining != 0 {
		t.Errorf("Remaining = %d, want 0", rateLimitErr.Remaining)
	}

	expectedReset := time.Unix(1700000000, 0).UTC()
	if rateLimitErr.Reset.Unix() != expectedReset.Unix() {
		t.Errorf("Reset = %v (unix: %d), want %v (unix: %d)", rateLimitErr.Reset, rateLimitErr.Reset.Unix(), expectedReset, expectedReset.Unix())
	}

	if rateLimitErr.RequestID != "req-456" {
		t.Errorf("RequestID = %q, want %q", rateLimitErr.RequestID, "req-456")
	}
}

func TestIsRetryable(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "network error",
			err:      NewNetworkError(errors.New("connection refused")),
			expected: true,
		},
		{
			name:     "timeout error",
			err:      NewTimeoutError(30*time.Second, errors.New("timeout")),
			expected: true,
		},
		{
			name: "server error (500)",
			err: &ServerError{APIError: &APIError{
				StatusCode: 500,
				Message:    "Internal error",
			}},
			expected: true,
		},
		{
			name: "rate limit error (429)",
			err: &RateLimitError{APIError: &APIError{
				StatusCode: 429,
				Message:    "Rate limit",
			}},
			expected: true,
		},
		{
			name: "authentication error (401)",
			err: &AuthenticationError{APIError: &APIError{
				StatusCode: 401,
				Message:    "Unauthorized",
			}},
			expected: false,
		},
		{
			name: "validation error (400)",
			err: &ValidationError{APIError: &APIError{
				StatusCode: 400,
				Message:    "Bad request",
			}},
			expected: false,
		},
		{
			name: "not found error (404)",
			err: &NotFoundError{APIError: &APIError{
				StatusCode: 404,
				Message:    "Not found",
			}},
			expected: false,
		},
		{
			name:     "circuit breaker open error",
			err:      NewCircuitBreakerOpenError(),
			expected: false,
		},
		{
			name:     "generic error",
			err:      errors.New("some error"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsRetryable(tt.err)
			if got != tt.expected {
				t.Errorf("IsRetryable(%v) = %v, want %v", tt.err, got, tt.expected)
			}
		})
	}
}

func TestAPIError_Error(t *testing.T) {
	tests := []struct {
		name     string
		err      *APIError
		expected string
	}{
		{
			name: "with code and message",
			err: &APIError{
				StatusCode: 400,
				Code:       "validation_error",
				Message:    "Invalid payload",
			},
			expected: "[400] validation_error: Invalid payload",
		},
		{
			name: "message only",
			err: &APIError{
				StatusCode: 500,
				Message:    "Server error",
			},
			expected: "[500] Server error",
		},
		{
			name: "code only",
			err: &APIError{
				StatusCode: 401,
				Code:       "unauthorized",
			},
			expected: "[401] unauthorized",
		},
		{
			name: "neither code nor message",
			err: &APIError{
				StatusCode: 500,
			},
			expected: "[500] unknown error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.err.Error()
			if got != tt.expected {
				t.Errorf("Error() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestAPIError_Unwrap(t *testing.T) {
	underlying := errors.New("underlying error")
	err := &APIError{
		StatusCode: 500,
		Message:    "Wrapper",
		Err:        underlying,
	}

	if !errors.Is(err, underlying) {
		t.Error("Expected Unwrap to expose underlying error")
	}
}

func TestAsAPIError(t *testing.T) {
	t.Run("extracts APIError", func(t *testing.T) {
		apiErr := &APIError{StatusCode: 500, Message: "test"}
		extracted, ok := AsAPIError(apiErr)
		if !ok {
			t.Fatal("Expected to extract APIError")
		}
		if extracted.StatusCode != 500 {
			t.Errorf("StatusCode = %d, want 500", extracted.StatusCode)
		}
	})

	t.Run("extracts from wrapped error", func(t *testing.T) {
		apiErr := &ServerError{APIError: &APIError{StatusCode: 500, Message: "test"}}
		extracted, ok := AsAPIError(apiErr)
		if !ok {
			t.Fatal("Expected to extract APIError from ServerError")
		}
		if extracted.StatusCode != 500 {
			t.Errorf("StatusCode = %d, want 500", extracted.StatusCode)
		}
	})

	t.Run("returns false for non-API error", func(t *testing.T) {
		_, ok := AsAPIError(errors.New("generic error"))
		if ok {
			t.Error("Expected AsAPIError to return false for generic error")
		}
	})
}
