package realtime

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// makeTestJWT builds an unsigned JWT whose payload carries the given expiry.
// jwtExpiry only reads the `exp` claim (no signature check), so the empty
// signature segment is sufficient for these tests.
func makeTestJWT(exp time.Time) string {
	enc := func(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }
	header := enc([]byte(`{"alg":"none","typ":"JWT"}`))
	payload := enc([]byte(fmt.Sprintf(`{"exp":%d}`, exp.Unix())))
	return header + "." + payload + "."
}

func TestReconnectBackoff_ClampsAndNeverZero(t *testing.T) {
	base := 1 * time.Second
	max := 30 * time.Second

	tests := []struct {
		attempt int
		want    time.Duration
	}{
		{attempt: 1, want: 1 * time.Second},
		{attempt: 2, want: 2 * time.Second},
		{attempt: 4, want: 8 * time.Second},
		{attempt: 6, want: max},  // 32s would exceed max -> capped
		{attempt: 64, want: max}, // would overflow the shift to 0 without clamping
		{attempt: 200, want: max},
	}

	for _, tt := range tests {
		got := reconnectBackoff(base, max, tt.attempt)
		if got != tt.want {
			t.Errorf("reconnectBackoff(attempt=%d) = %v, want %v", tt.attempt, got, tt.want)
		}
		if got <= 0 {
			t.Errorf("reconnectBackoff(attempt=%d) produced non-positive delay %v (reconnect storm)", tt.attempt, got)
		}
	}
}

func TestAppendQueryParam(t *testing.T) {
	got, err := appendQueryParam("wss://api.spooled.cloud/api/v1/ws", "token", "jwt abc/xyz+123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	u, err := url.Parse(got)
	if err != nil {
		t.Fatalf("result is not a valid URL: %v", err)
	}
	if u.Query().Get("token") != "jwt abc/xyz+123" {
		t.Errorf("token query param not round-tripped, got %q", u.Query().Get("token"))
	}
}

func TestSSE_BuildURL_APIKeyGoesInQuery(t *testing.T) {
	// With only an API key, it must be sent as ?api_key= (never X-API-Key).
	c := NewSSEClient(ConnectionOptions{
		BaseURL: "https://api.spooled.cloud",
		APIKey:  "sp_live_secret",
	})
	got := c.buildSSEURL()
	u, err := url.Parse(got)
	if err != nil {
		t.Fatalf("invalid SSE URL: %v", err)
	}
	if u.Query().Get("api_key") != "sp_live_secret" {
		t.Errorf("expected api_key query param, got URL %q", got)
	}

	// With a bearer token configured, the key must NOT be duplicated in the query.
	c2 := NewSSEClient(ConnectionOptions{
		BaseURL: "https://api.spooled.cloud",
		Token:   "jwt-token",
		APIKey:  "sp_live_secret",
	})
	got2 := c2.buildSSEURL()
	if strings.Contains(got2, "api_key") {
		t.Errorf("expected no api_key in query when token is set, got %q", got2)
	}
}

func TestRealtimeLogin_ExchangesAPIKeyForJWT(t *testing.T) {
	var gotBody map[string]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/auth/login" {
			t.Errorf("unexpected login path: %s", r.URL.Path)
		}
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "jwt-from-login",
			"refresh_token": "refresh-xyz",
			"token_type":    "Bearer",
			"expires_in":    3600,
		})
	}))
	defer server.Close()

	token, err := realtimeLogin(context.Background(), server.URL, "sp_live_secret")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "jwt-from-login" {
		t.Errorf("expected access token from login, got %q", token)
	}
	if gotBody["api_key"] != "sp_live_secret" {
		t.Errorf("expected api_key in login body, got %v", gotBody)
	}
}

func TestWebSocket_ResolveToken_LoginWithAPIKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"access_token": "jwt-abc"})
	}))
	defer server.Close()

	// API-key client: resolveToken must log in to obtain a JWT.
	c := NewWebSocketClient(ConnectionOptions{
		BaseURL: server.URL,
		WSURL:   "ws://example.invalid/api/v1/ws",
		APIKey:  "sp_live_secret",
	})
	token, err := c.resolveToken(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "jwt-abc" {
		t.Errorf("expected JWT from login, got %q", token)
	}

	// JWT client: resolveToken uses the configured token directly (no login).
	c2 := NewWebSocketClient(ConnectionOptions{
		BaseURL: "http://example.invalid",
		WSURL:   "ws://example.invalid/api/v1/ws",
		Token:   "already-a-jwt",
	})
	token2, err := c2.resolveToken(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token2 != "already-a-jwt" {
		t.Errorf("expected configured token to be used directly, got %q", token2)
	}
}

func TestWebSocket_ResolveToken_CachesJWTAcrossReconnects(t *testing.T) {
	var logins int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&logins, 1)
		// A JWT that expires well beyond the refresh leeway.
		json.NewEncoder(w).Encode(map[string]any{
			"access_token": makeTestJWT(time.Now().Add(time.Hour)),
		})
	}))
	defer server.Close()

	c := NewWebSocketClient(ConnectionOptions{
		BaseURL: server.URL,
		WSURL:   "ws://example.invalid/api/v1/ws",
		APIKey:  "sp_live_secret",
	})

	// Two resolutions within the token's lifetime must perform exactly ONE
	// login; the second reuses the cached JWT.
	first, err := c.resolveToken(context.Background())
	if err != nil {
		t.Fatalf("first resolveToken: %v", err)
	}
	second, err := c.resolveToken(context.Background())
	if err != nil {
		t.Fatalf("second resolveToken: %v", err)
	}
	if first != second {
		t.Errorf("expected cached token to be reused, got %q then %q", first, second)
	}
	if n := atomic.LoadInt32(&logins); n != 1 {
		t.Fatalf("expected exactly ONE login within token lifetime, got %d", n)
	}
}

func TestWebSocket_ResolveToken_ReloginsWhenCachedTokenExpired(t *testing.T) {
	var logins int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&logins, 1)
		// Already-expired JWT: every resolution must re-login.
		json.NewEncoder(w).Encode(map[string]any{
			"access_token": makeTestJWT(time.Now().Add(-time.Minute)),
		})
	}))
	defer server.Close()

	c := NewWebSocketClient(ConnectionOptions{
		BaseURL: server.URL,
		WSURL:   "ws://example.invalid/api/v1/ws",
		APIKey:  "sp_live_secret",
	})

	if _, err := c.resolveToken(context.Background()); err != nil {
		t.Fatalf("first resolveToken: %v", err)
	}
	if _, err := c.resolveToken(context.Background()); err != nil {
		t.Fatalf("second resolveToken: %v", err)
	}
	if n := atomic.LoadInt32(&logins); n != 2 {
		t.Fatalf("expected an expired cached token to trigger re-login (2 logins), got %d", n)
	}
}

func TestWebSocket_ResolveToken_ConcurrentReconnectsCoalesceLogin(t *testing.T) {
	// Simulate a reconnect storm: many goroutines resolve the token at once.
	// They must coalesce into a single login instead of stampeding /auth/login.
	var logins int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&logins, 1)
		json.NewEncoder(w).Encode(map[string]any{
			"access_token": makeTestJWT(time.Now().Add(time.Hour)),
		})
	}))
	defer server.Close()

	c := NewWebSocketClient(ConnectionOptions{
		BaseURL: server.URL,
		WSURL:   "ws://example.invalid/api/v1/ws",
		APIKey:  "sp_live_secret",
	})

	const goroutines = 25
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			if _, err := c.resolveToken(context.Background()); err != nil {
				t.Errorf("resolveToken: %v", err)
			}
		}()
	}
	wg.Wait()

	if n := atomic.LoadInt32(&logins); n != 1 {
		t.Fatalf("expected concurrent reconnects to coalesce into ONE login, got %d", n)
	}
}

func TestWebSocket_DisconnectCancelsPendingReconnect(t *testing.T) {
	c := NewWebSocketClient(ConnectionOptions{
		WSURL:                "ws://example.invalid/api/v1/ws",
		APIKey:               "sp_live_secret",
		AutoReconnect:        true,
		ReconnectDelay:       10 * time.Second, // long enough not to fire during the test
		MaxReconnectDelay:    30 * time.Second,
		MaxReconnectAttempts: 5,
	})

	// Simulate an active connection that then drops, scheduling a reconnect.
	c.mu.Lock()
	c.state = StateConnected
	c.mu.Unlock()
	c.handleDisconnect()

	c.mu.RLock()
	scheduled := c.reconnectTimer
	c.mu.RUnlock()
	if scheduled == nil {
		t.Fatal("expected a reconnect timer to be scheduled")
	}

	// The user explicitly disconnects; the pending reconnect must be cancelled.
	if err := c.Disconnect(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.State() != StateDisconnected {
		t.Errorf("expected state Disconnected after Disconnect(), got %v", c.State())
	}
	c.mu.RLock()
	remaining := c.reconnectTimer
	c.mu.RUnlock()
	if remaining != nil {
		t.Error("expected reconnect timer to be cleared after Disconnect()")
	}
}

func TestSSE_DisconnectCancelsPendingReconnect(t *testing.T) {
	c := NewSSEClient(ConnectionOptions{
		BaseURL:              "https://api.spooled.cloud",
		APIKey:               "sp_live_secret",
		AutoReconnect:        true,
		ReconnectDelay:       10 * time.Second,
		MaxReconnectDelay:    30 * time.Second,
		MaxReconnectAttempts: 5,
	})

	c.mu.Lock()
	c.state = StateConnected
	c.mu.Unlock()
	c.handleDisconnect()

	c.mu.RLock()
	scheduled := c.reconnectTimer
	c.mu.RUnlock()
	if scheduled == nil {
		t.Fatal("expected a reconnect timer to be scheduled")
	}

	if err := c.Disconnect(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.State() != StateDisconnected {
		t.Errorf("expected state Disconnected after Disconnect(), got %v", c.State())
	}
	c.mu.RLock()
	remaining := c.reconnectTimer
	c.mu.RUnlock()
	if remaining != nil {
		t.Error("expected reconnect timer to be cleared after Disconnect()")
	}
}
