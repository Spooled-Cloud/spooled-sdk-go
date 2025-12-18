package httpx

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestTransport_Do_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Request-ID", "test-req-id")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	defer server.Close()

	transport := NewTransport(Config{
		BaseURL: server.URL,
		APIKey:  "sp_test_123456789012345678901234567890",
	})

	resp, err := transport.Do(context.Background(), &Request{
		Method: http.MethodGet,
		Path:   "/test",
	})

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if resp.StatusCode != 200 {
		t.Errorf("StatusCode = %d, want 200", resp.StatusCode)
	}

	if resp.RequestID != "test-req-id" {
		t.Errorf("RequestID = %q, want %q", resp.RequestID, "test-req-id")
	}
}

func TestTransport_Do_WithBody(t *testing.T) {
	var receivedBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &receivedBody)
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	transport := NewTransport(Config{
		BaseURL: server.URL,
		APIKey:  "sp_test_123456789012345678901234567890",
	})

	_, err := transport.Do(context.Background(), &Request{
		Method: http.MethodPost,
		Path:   "/jobs",
		Body: map[string]any{
			"queue_name": "test-queue",
			"payload":    map[string]string{"key": "value"},
		},
	})

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if receivedBody["queue_name"] != "test-queue" {
		t.Errorf("Received body missing queue_name")
	}
}

func TestTransport_Do_AuthHeaders(t *testing.T) {
	tests := []struct {
		name        string
		config      Config
		useAdminKey bool
		expectKey   string
		expectValue string
	}{
		{
			name: "API key header",
			config: Config{
				APIKey: "sp_test_123456789012345678901234567890",
			},
			expectKey:   "Authorization",
			expectValue: "Bearer sp_test_123456789012345678901234567890",
		},
		{
			name: "Access token header",
			config: Config{
				AccessToken: "jwt-token-here",
			},
			expectKey:   "Authorization",
			expectValue: "Bearer jwt-token-here",
		},
		{
			name: "Admin key header",
			config: Config{
				APIKey:   "sp_test_123456789012345678901234567890",
				AdminKey: "admin-key-here",
			},
			useAdminKey: true,
			expectKey:   "X-Admin-Key",
			expectValue: "admin-key-here",
		},
		{
			name: "Access token takes precedence over API key",
			config: Config{
				APIKey:      "sp_test_123456789012345678901234567890",
				AccessToken: "jwt-token-here",
			},
			expectKey:   "Authorization",
			expectValue: "Bearer jwt-token-here",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var receivedHeaders http.Header
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				receivedHeaders = r.Header.Clone()
				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()

			tt.config.BaseURL = server.URL
			transport := NewTransport(tt.config)

			_, err := transport.Do(context.Background(), &Request{
				Method:      http.MethodGet,
				Path:        "/test",
				UseAdminKey: tt.useAdminKey,
			})

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			got := receivedHeaders.Get(tt.expectKey)
			if got != tt.expectValue {
				t.Errorf("Header %q = %q, want %q", tt.expectKey, got, tt.expectValue)
			}
		})
	}
}

func TestTransport_Do_CustomHeaders(t *testing.T) {
	var receivedHeaders http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeaders = r.Header.Clone()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	transport := NewTransport(Config{
		BaseURL: server.URL,
		APIKey:  "sp_test_123456789012345678901234567890",
		Headers: map[string]string{
			"X-Custom-Header": "custom-value",
		},
	})

	_, err := transport.Do(context.Background(), &Request{
		Method: http.MethodGet,
		Path:   "/test",
		Headers: map[string]string{
			"X-Request-Header": "request-value",
		},
	})

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if receivedHeaders.Get("X-Custom-Header") != "custom-value" {
		t.Errorf("Missing custom header from config")
	}

	if receivedHeaders.Get("X-Request-Header") != "request-value" {
		t.Errorf("Missing request-specific header")
	}
}

func TestTransport_Do_UserAgent(t *testing.T) {
	var receivedUA string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedUA = r.Header.Get("User-Agent")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	transport := NewTransport(Config{
		BaseURL: server.URL,
		APIKey:  "sp_test_123456789012345678901234567890",
	})

	transport.Do(context.Background(), &Request{
		Method: http.MethodGet,
		Path:   "/test",
	})

	if !strings.Contains(receivedUA, "spooled-go") {
		t.Errorf("User-Agent should contain 'spooled-go', got %q", receivedUA)
	}
}

func TestTransport_Do_QueryParams(t *testing.T) {
	var receivedQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedQuery = r.URL.RawQuery
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	transport := NewTransport(Config{
		BaseURL: server.URL,
		APIKey:  "sp_test_123456789012345678901234567890",
	})

	_, err := transport.Do(context.Background(), &Request{
		Method: http.MethodGet,
		Path:   "/jobs",
		Query: map[string]string{
			"status": "pending",
			"limit":  "10",
		},
	})

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if !strings.Contains(receivedQuery, "status=pending") {
		t.Errorf("Query should contain 'status=pending', got %q", receivedQuery)
	}
	if !strings.Contains(receivedQuery, "limit=10") {
		t.Errorf("Query should contain 'limit=10', got %q", receivedQuery)
	}
}

func TestTransport_Do_ErrorResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{
			"code":    "not_found",
			"message": "Job not found",
		})
	}))
	defer server.Close()

	transport := NewTransport(Config{
		BaseURL: server.URL,
		APIKey:  "sp_test_123456789012345678901234567890",
	})

	_, err := transport.Do(context.Background(), &Request{
		Method: http.MethodGet,
		Path:   "/jobs/123",
	})

	if err == nil {
		t.Fatal("Expected error for 404 response")
	}

	if !IsNotFoundError(err) {
		t.Errorf("Expected NotFoundError, got %T", err)
	}
}

func TestTransport_Do_Retry(t *testing.T) {
	var requestCount int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&requestCount, 1)
		if count < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	transport := NewTransport(Config{
		BaseURL: server.URL,
		APIKey:  "sp_test_123456789012345678901234567890",
		Retry: RetryConfig{
			MaxRetries: 3,
			BaseDelay:  1 * time.Millisecond,
			Jitter:     false,
		},
	})

	_, err := transport.Do(context.Background(), &Request{
		Method: http.MethodGet,
		Path:   "/test",
	})

	if err != nil {
		t.Fatalf("Expected success after retries, got error: %v", err)
	}

	if requestCount != 3 {
		t.Errorf("Expected 3 requests (initial + 2 retries), got %d", requestCount)
	}
}

func TestTransport_Do_NoRetryForPOST(t *testing.T) {
	var requestCount int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requestCount, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	transport := NewTransport(Config{
		BaseURL: server.URL,
		APIKey:  "sp_test_123456789012345678901234567890",
		Retry: RetryConfig{
			MaxRetries: 3,
			BaseDelay:  1 * time.Millisecond,
			Jitter:     false,
		},
	})

	_, err := transport.Do(context.Background(), &Request{
		Method: http.MethodPost,
		Path:   "/jobs",
		Body:   map[string]string{"queue_name": "test"},
	})

	if err == nil {
		t.Fatal("Expected error")
	}

	// POST without Idempotent flag should not retry
	if requestCount != 1 {
		t.Errorf("Expected 1 request (no retries for POST), got %d", requestCount)
	}
}

func TestTransport_Do_RetryIdempotentPOST(t *testing.T) {
	var requestCount int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&requestCount, 1)
		if count < 2 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	transport := NewTransport(Config{
		BaseURL: server.URL,
		APIKey:  "sp_test_123456789012345678901234567890",
		Retry: RetryConfig{
			MaxRetries: 3,
			BaseDelay:  1 * time.Millisecond,
			Jitter:     false,
		},
	})

	_, err := transport.Do(context.Background(), &Request{
		Method:     http.MethodPost,
		Path:       "/jobs",
		Body:       map[string]string{"queue_name": "test"},
		Idempotent: true,
	})

	if err != nil {
		t.Fatalf("Expected success after retry, got error: %v", err)
	}

	if requestCount != 2 {
		t.Errorf("Expected 2 requests (initial + 1 retry), got %d", requestCount)
	}
}

func TestTransport_Do_CircuitBreaker(t *testing.T) {
	var requestCount int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requestCount, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	transport := NewTransport(Config{
		BaseURL: server.URL,
		APIKey:  "sp_test_123456789012345678901234567890",
		Retry: RetryConfig{
			MaxRetries: 0, // No retries to speed up test
			BaseDelay:  1 * time.Millisecond, // Must set to indicate config is provided
			Jitter:     false,
		},
		CircuitBreaker: CircuitBreakerConfig{
			Enabled:          true,
			FailureThreshold: 2,
			SuccessThreshold: 1,
			Timeout:          100 * time.Millisecond,
		},
	})

	// First two requests should fail and open circuit
	transport.Do(context.Background(), &Request{Method: http.MethodGet, Path: "/test"})
	transport.Do(context.Background(), &Request{Method: http.MethodGet, Path: "/test"})

	if requestCount != 2 {
		t.Errorf("Expected 2 requests, got %d", requestCount)
	}

	// Third request should be blocked by circuit breaker
	_, err := transport.Do(context.Background(), &Request{Method: http.MethodGet, Path: "/test"})

	if requestCount != 2 {
		t.Errorf("Expected circuit breaker to block request, got %d requests", requestCount)
	}

	var cbErr *CircuitBreakerOpenError
	if err == nil || !strings.Contains(err.Error(), "circuit breaker is open") {
		t.Errorf("Expected CircuitBreakerOpenError, got %v", err)
	}
	_ = cbErr
}

func TestTransport_Do_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(1 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	transport := NewTransport(Config{
		BaseURL: server.URL,
		APIKey:  "sp_test_123456789012345678901234567890",
		Timeout: 5 * time.Second,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := transport.Do(ctx, &Request{
		Method: http.MethodGet,
		Path:   "/slow",
	})

	if err == nil {
		t.Fatal("Expected error from context cancellation")
	}
}

func TestTransport_SetAccessToken(t *testing.T) {
	var receivedAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	transport := NewTransport(Config{
		BaseURL: server.URL,
		APIKey:  "sp_test_123456789012345678901234567890",
	})

	// Initially uses API key (via Bearer token)
	transport.Do(context.Background(), &Request{Method: http.MethodGet, Path: "/test"})
	if receivedAuth != "Bearer sp_test_123456789012345678901234567890" {
		t.Errorf("Expected API key as Bearer token, got %q", receivedAuth)
	}

	// Update access token
	transport.SetAccessToken("new-jwt-token")

	transport.Do(context.Background(), &Request{Method: http.MethodGet, Path: "/test"})
	if receivedAuth != "Bearer new-jwt-token" {
		t.Errorf("Expected new Bearer token, got %q", receivedAuth)
	}
}

func TestJSON(t *testing.T) {
	resp := &Response{
		StatusCode: 200,
		Body:       []byte(`{"id":"123","name":"test"}`),
	}

	type Result struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}

	result, err := JSON[Result](resp)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if result.ID != "123" {
		t.Errorf("ID = %q, want %q", result.ID, "123")
	}
	if result.Name != "test" {
		t.Errorf("Name = %q, want %q", result.Name, "test")
	}
}

func TestJSONArray(t *testing.T) {
	resp := &Response{
		StatusCode: 200,
		Body:       []byte(`[{"id":"1"},{"id":"2"}]`),
	}

	type Item struct {
		ID string `json:"id"`
	}

	result, err := JSONArray[Item](resp)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if len(result) != 2 {
		t.Errorf("len(result) = %d, want 2", len(result))
	}
}

