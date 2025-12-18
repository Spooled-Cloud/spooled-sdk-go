package httpx

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// TokenRefresher handles automatic token refresh.
type TokenRefresher struct {
	mu         sync.Mutex
	refreshing bool
	done       chan struct{}

	baseURL      string
	apiKey       string
	refreshToken string
	accessToken  string
	expiresAt    time.Time

	client *http.Client
	logger Logger
}

// NewTokenRefresher creates a new token refresher.
func NewTokenRefresher(baseURL, apiKey, refreshToken, accessToken string, logger Logger) *TokenRefresher {
	return &TokenRefresher{
		baseURL:      strings.TrimSuffix(baseURL, "/"),
		apiKey:       apiKey,
		refreshToken: refreshToken,
		accessToken:  accessToken,
		client:       &http.Client{Timeout: 30 * time.Second},
		logger:       logger,
	}
}

// SetAccessToken updates the access token.
func (tr *TokenRefresher) SetAccessToken(token string, expiresIn int) {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	tr.accessToken = token
	if expiresIn > 0 {
		// Set expiry with some buffer (refresh 1 minute before actual expiry)
		tr.expiresAt = time.Now().Add(time.Duration(expiresIn-60) * time.Second)
	}
}

// SetRefreshToken updates the refresh token.
func (tr *TokenRefresher) SetRefreshToken(token string) {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	tr.refreshToken = token
}

// GetAccessToken returns the current access token.
func (tr *TokenRefresher) GetAccessToken() string {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	return tr.accessToken
}

// NeedsRefresh returns true if the token needs to be refreshed.
func (tr *TokenRefresher) NeedsRefresh() bool {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	if tr.accessToken == "" {
		return false
	}
	if tr.expiresAt.IsZero() {
		return false
	}
	return time.Now().After(tr.expiresAt)
}

// RefreshIfNeeded refreshes the token if it's expired or about to expire.
// Uses single-flight to prevent multiple concurrent refreshes.
func (tr *TokenRefresher) RefreshIfNeeded(ctx context.Context) error {
	if !tr.NeedsRefresh() {
		return nil
	}
	return tr.Refresh(ctx)
}

// Refresh performs a token refresh with single-flight coordination.
// Only one goroutine will actually perform the refresh; others will wait for it.
func (tr *TokenRefresher) Refresh(ctx context.Context) error {
	return tr.refreshInternal(ctx, false)
}

// ForceRefresh performs a token refresh regardless of expiry status.
// Used when receiving a 401 response.
func (tr *TokenRefresher) ForceRefresh(ctx context.Context) error {
	return tr.refreshInternal(ctx, true)
}

// refreshInternal performs the actual refresh with optional force flag.
func (tr *TokenRefresher) refreshInternal(ctx context.Context, force bool) error {
	tr.mu.Lock()

	// If another goroutine is already refreshing, wait for it
	if tr.refreshing {
		done := tr.done
		tr.mu.Unlock()

		// Wait for refresh to complete
		select {
		case <-done:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	// Check if token was already refreshed by another goroutine (unless forced)
	if !force && !tr.needsRefreshLocked() && tr.accessToken != "" {
		tr.mu.Unlock()
		return nil
	}

	// Mark as refreshing and create done channel
	tr.refreshing = true
	tr.done = make(chan struct{})
	refreshToken := tr.refreshToken
	apiKey := tr.apiKey
	tr.mu.Unlock()

	// Perform refresh
	var err error
	defer func() {
		tr.mu.Lock()
		tr.refreshing = false
		close(tr.done)
		tr.mu.Unlock()
	}()

	if refreshToken != "" {
		err = tr.refreshWithToken(ctx, refreshToken)
	} else if apiKey != "" {
		err = tr.refreshWithAPIKey(ctx, apiKey)
	} else {
		return fmt.Errorf("no refresh token or API key available")
	}

	return err
}

// needsRefreshLocked returns true if the token needs refresh (must be called with lock held).
func (tr *TokenRefresher) needsRefreshLocked() bool {
	if tr.accessToken == "" {
		return false
	}
	if tr.expiresAt.IsZero() {
		return false
	}
	return time.Now().After(tr.expiresAt)
}

// refreshWithToken refreshes using a refresh token.
func (tr *TokenRefresher) refreshWithToken(ctx context.Context, refreshToken string) error {
	tr.log("refreshing token using refresh_token")

	body := fmt.Sprintf(`{"refresh_token":"%s"}`, refreshToken)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		tr.baseURL+"/api/v1/auth/refresh",
		strings.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create refresh request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := tr.client.Do(req)
	if err != nil {
		return fmt.Errorf("refresh request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// If refresh token is invalid, try API key if available
		if resp.StatusCode == http.StatusUnauthorized && tr.apiKey != "" {
			return tr.refreshWithAPIKey(ctx, tr.apiKey)
		}
		return fmt.Errorf("refresh failed with status %d", resp.StatusCode)
	}

	var result struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("failed to decode refresh response: %w", err)
	}

	tr.SetAccessToken(result.AccessToken, result.ExpiresIn)
	tr.log("token refreshed successfully", "expires_in", result.ExpiresIn)

	return nil
}

// refreshWithAPIKey performs a fresh login using the API key.
func (tr *TokenRefresher) refreshWithAPIKey(ctx context.Context, apiKey string) error {
	tr.log("refreshing token using api_key (re-login)")

	body := fmt.Sprintf(`{"api_key":"%s"}`, apiKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		tr.baseURL+"/api/v1/auth/login",
		strings.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create login request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := tr.client.Do(req)
	if err != nil {
		return fmt.Errorf("login request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("login failed with status %d", resp.StatusCode)
	}

	var result struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("failed to decode login response: %w", err)
	}

	tr.SetAccessToken(result.AccessToken, result.ExpiresIn)
	tr.SetRefreshToken(result.RefreshToken)
	tr.log("re-login successful", "expires_in", result.ExpiresIn)

	return nil
}

// log logs a debug message.
func (tr *TokenRefresher) log(msg string, keysAndValues ...any) {
	if tr.logger != nil {
		tr.logger.Debug(msg, keysAndValues...)
	}
}
