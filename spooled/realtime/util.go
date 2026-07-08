package realtime

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// realtimeHTTPClient is used for the short-lived login exchange. Realtime
// connections manage their own long-lived transports, so this client only
// needs a modest timeout for the login round-trip.
var realtimeHTTPClient = &http.Client{Timeout: 30 * time.Second}

// tokenLeeway is how long before a JWT's real expiry we treat it as stale, so
// we refresh proactively instead of racing an expiry mid-(re)connect.
const tokenLeeway = 60 * time.Second

// tokenCache holds a JWT obtained from the login endpoint together with its
// decoded expiry, so WebSocket reconnects can reuse it instead of logging in
// on every attempt. Its own mutex makes it safe for concurrent use.
type tokenCache struct {
	mu    sync.Mutex
	token string
	// exp is the JWT `exp` claim; zero means the expiry could not be decoded,
	// in which case the token is treated as usable until the server rejects it.
	exp time.Time
}

// getOrLogin returns a cached JWT if one is present and not at/near expiry;
// otherwise it calls loginFn and caches the result. The lock is held across
// loginFn so that a reconnect storm coalesces into a single login request
// rather than stampeding the rate-limited /auth/login endpoint into a 429.
func (tc *tokenCache) getOrLogin(now time.Time, loginFn func() (string, error)) (string, error) {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	if tc.token != "" && (tc.exp.IsZero() || now.Before(tc.exp.Add(-tokenLeeway))) {
		return tc.token, nil
	}

	token, err := loginFn()
	if err != nil {
		return "", err
	}
	tc.token = token
	tc.exp, _ = jwtExpiry(token)
	return token, nil
}

// invalidate clears any cached token so the next getOrLogin re-logs in. It is
// called when the server rejects the token (e.g. a 401/403 on the WS upgrade).
func (tc *tokenCache) invalidate() {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	tc.token = ""
	tc.exp = time.Time{}
}

// jwtExpiry decodes the `exp` claim from a JWT WITHOUT verifying its signature.
// It base64url-decodes the payload segment and reads `exp` (seconds since the
// Unix epoch). The boolean reports whether an expiry was successfully decoded.
func jwtExpiry(token string) (time.Time, bool) {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return time.Time{}, false
	}
	// JWT segments are base64url without padding; tolerate padding just in case.
	payload, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(parts[1], "="))
	if err != nil {
		return time.Time{}, false
	}
	var claims struct {
		Exp float64 `json:"exp"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return time.Time{}, false
	}
	if claims.Exp <= 0 {
		return time.Time{}, false
	}
	return time.Unix(int64(claims.Exp), 0), true
}

// reconnectBackoff computes the delay before the next reconnect attempt using
// exponential backoff, clamped so it can never overflow to zero or a negative
// duration (which would cause a reconnect storm) and never exceeds max.
//
// attempt is 1-based (the first reconnect is attempt 1).
func reconnectBackoff(base, max time.Duration, attempt int) time.Duration {
	if base <= 0 {
		base = time.Second
	}
	if max <= 0 {
		max = 30 * time.Second
	}
	if attempt < 1 {
		attempt = 1
	}

	// Clamp the shift exponent: 1<<shift on a signed int overflows to a
	// negative value around shift 63 and to 0 at >= 64. Capping at 30 keeps the
	// multiplier well within range while still reaching very long delays that
	// are then bounded by max.
	shift := attempt - 1
	if shift > 30 {
		shift = 30
	}

	delay := base * time.Duration(1<<uint(shift))
	// Guard against overflow producing a non-positive delay, and enforce the
	// configured ceiling.
	if delay <= 0 || delay > max {
		delay = max
	}
	return delay
}

// realtimeLogin exchanges an API key for a JWT access token via the data-plane
// login endpoint (POST /api/v1/auth/login). The WebSocket endpoint
// authenticates ONLY via a JWT in the ?token= query parameter, so an API-key
// client must obtain a JWT first.
func realtimeLogin(ctx context.Context, baseURL, apiKey string) (string, error) {
	if baseURL == "" {
		baseURL = DefaultConnectionOptions().BaseURL
	}
	endpoint := strings.TrimSuffix(baseURL, "/") + "/api/v1/auth/login"

	payload, err := json.Marshal(map[string]string{"api_key": apiKey})
	if err != nil {
		return "", fmt.Errorf("failed to encode login request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("failed to create login request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := realtimeHTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("login request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("login failed with status %d", resp.StatusCode)
	}

	var out struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("failed to decode login response: %w", err)
	}
	if out.AccessToken == "" {
		return "", fmt.Errorf("login response did not contain an access_token")
	}
	return out.AccessToken, nil
}

// appendQueryParam returns rawURL with an additional query parameter set,
// preserving any existing query parameters and encoding correctly.
func appendQueryParam(rawURL, key, value string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("invalid url %q: %w", rawURL, err)
	}
	q := u.Query()
	q.Set(key, value)
	u.RawQuery = q.Encode()
	return u.String(), nil
}
