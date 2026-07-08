package realtime

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// realtimeHTTPClient is used for the short-lived login exchange. Realtime
// connections manage their own long-lived transports, so this client only
// needs a modest timeout for the login round-trip.
var realtimeHTTPClient = &http.Client{Timeout: 30 * time.Second}

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
