package httpx

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestTokenRefresher_RefreshWithToken(t *testing.T) {
	refreshCount := int32(0)
	
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/auth/refresh" {
			atomic.AddInt32(&refreshCount, 1)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{
				"access_token": "new-access-token",
				"expires_in":   3600,
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	tr := NewTokenRefresher(server.URL, "", "refresh-token", "old-access-token", nil)
	tr.expiresAt = time.Now().Add(-1 * time.Hour) // Expired

	err := tr.Refresh(context.Background())
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if atomic.LoadInt32(&refreshCount) != 1 {
		t.Errorf("Expected 1 refresh call, got %d", refreshCount)
	}

	if tr.GetAccessToken() != "new-access-token" {
		t.Errorf("Expected access token to be updated")
	}
}

func TestTokenRefresher_RefreshWithAPIKey(t *testing.T) {
	loginCount := int32(0)
	
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/auth/login" {
			atomic.AddInt32(&loginCount, 1)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{
				"access_token":  "new-access-token",
				"refresh_token": "new-refresh-token",
				"expires_in":    3600,
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	tr := NewTokenRefresher(server.URL, "sp_test_apikey", "", "", nil)
	tr.expiresAt = time.Now().Add(-1 * time.Hour) // Expired

	err := tr.Refresh(context.Background())
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if atomic.LoadInt32(&loginCount) != 1 {
		t.Errorf("Expected 1 login call, got %d", loginCount)
	}

	if tr.GetAccessToken() != "new-access-token" {
		t.Errorf("Expected access token to be updated")
	}
}

func TestTokenRefresher_SingleFlight(t *testing.T) {
	refreshCount := int32(0)
	var wg sync.WaitGroup
	
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/auth/refresh" {
			atomic.AddInt32(&refreshCount, 1)
			// Simulate slow refresh
			time.Sleep(100 * time.Millisecond)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{
				"access_token": "new-access-token",
				"expires_in":   3600,
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	tr := NewTokenRefresher(server.URL, "", "refresh-token", "old-access-token", nil)
	tr.expiresAt = time.Now().Add(-1 * time.Hour) // Expired

	// Start 10 concurrent refresh attempts
	numGoroutines := 10
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			err := tr.Refresh(context.Background())
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		}()
	}

	wg.Wait()

	// Despite 10 concurrent calls, only 1 refresh should have been made
	count := atomic.LoadInt32(&refreshCount)
	if count != 1 {
		t.Errorf("Expected 1 refresh call (single-flight), got %d", count)
	}
}

func TestTokenRefresher_NeedsRefresh(t *testing.T) {
	tr := NewTokenRefresher("http://example.com", "", "", "access-token", nil)

	// No expiry set - shouldn't need refresh
	if tr.NeedsRefresh() {
		t.Error("Shouldn't need refresh when no expiry is set")
	}

	// Set future expiry
	tr.SetAccessToken("token", 3600)
	if tr.NeedsRefresh() {
		t.Error("Shouldn't need refresh when token is not expired")
	}

	// Set past expiry
	tr.expiresAt = time.Now().Add(-1 * time.Hour)
	if !tr.NeedsRefresh() {
		t.Error("Should need refresh when token is expired")
	}
}

func TestTokenRefresher_FallbackToAPIKey(t *testing.T) {
	refreshCount := int32(0)
	loginCount := int32(0)
	
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/auth/refresh" {
			atomic.AddInt32(&refreshCount, 1)
			// Refresh fails with 401
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.URL.Path == "/api/v1/auth/login" {
			atomic.AddInt32(&loginCount, 1)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{
				"access_token":  "new-access-token",
				"refresh_token": "new-refresh-token",
				"expires_in":    3600,
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	// Has both refresh token and API key
	tr := NewTokenRefresher(server.URL, "sp_test_apikey", "invalid-refresh-token", "old-access-token", nil)
	tr.expiresAt = time.Now().Add(-1 * time.Hour) // Expired

	err := tr.Refresh(context.Background())
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Should have tried refresh first, then fallen back to login
	if atomic.LoadInt32(&refreshCount) != 1 {
		t.Errorf("Expected 1 refresh attempt, got %d", refreshCount)
	}
	if atomic.LoadInt32(&loginCount) != 1 {
		t.Errorf("Expected 1 login attempt (fallback), got %d", loginCount)
	}

	if tr.GetAccessToken() != "new-access-token" {
		t.Errorf("Expected access token to be updated")
	}
}

func TestTransport_AutoRefresh401(t *testing.T) {
	requestCount := int32(0)
	refreshCount := int32(0)
	
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/auth/refresh" {
			atomic.AddInt32(&refreshCount, 1)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{
				"access_token": "new-access-token",
				"expires_in":   3600,
			})
			return
		}
		
		if r.URL.Path == "/api/v1/test" {
			count := atomic.AddInt32(&requestCount, 1)
			// First request returns 401, second request succeeds
			if count == 1 {
				w.WriteHeader(http.StatusUnauthorized)
				json.NewEncoder(w).Encode(map[string]string{"error": "token expired"})
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
			return
		}
		
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	transport := NewTransport(Config{
		BaseURL:          server.URL,
		AccessToken:      "expired-token",
		RefreshToken:     "valid-refresh-token",
		AutoRefreshToken: true,
		Retry: RetryConfig{
			MaxRetries: 0,
			BaseDelay:  1 * time.Millisecond,
		},
	})

	_, err := transport.Do(context.Background(), &Request{
		Method: http.MethodGet,
		Path:   "/api/v1/test",
	})

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Should have made 2 requests (first 401, then retry after refresh)
	if atomic.LoadInt32(&requestCount) != 2 {
		t.Errorf("Expected 2 requests, got %d", requestCount)
	}

	// Should have refreshed token once
	if atomic.LoadInt32(&refreshCount) != 1 {
		t.Errorf("Expected 1 refresh, got %d", refreshCount)
	}
}


