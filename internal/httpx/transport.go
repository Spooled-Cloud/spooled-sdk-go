// Package httpx provides HTTP transport utilities for the Spooled SDK.
package httpx

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/spooled-cloud/spooled-sdk-go/internal/version"
)

// Transport wraps an http.Client with retry, circuit breaker, and auth handling.
type Transport struct {
	client  *http.Client
	baseURL string
	apiKey  string
	// tokenMu guards accessToken, which is mutated by token refresh and read
	// concurrently by every request goroutine sharing this Transport.
	tokenMu          sync.RWMutex
	accessToken      string
	adminKey         string
	userAgent        string
	headers          map[string]string
	retry            *RetryPolicy
	circuitBreaker   *CircuitBreaker
	logger           Logger
	tokenRefresher   *TokenRefresher
	autoRefreshToken bool
}

// Logger is an interface for debug logging.
type Logger interface {
	Debug(msg string, keysAndValues ...any)
}

// Config holds configuration for the transport.
type Config struct {
	BaseURL          string
	APIKey           string
	AccessToken      string
	RefreshToken     string
	AdminKey         string
	UserAgent        string
	Headers          map[string]string
	Timeout          time.Duration
	Retry            RetryConfig
	CircuitBreaker   CircuitBreakerConfig
	Logger           Logger
	AutoRefreshToken bool
}

// RetryConfig configures retry behavior.
type RetryConfig struct {
	MaxRetries int
	BaseDelay  time.Duration
	MaxDelay   time.Duration
	Factor     float64
	Jitter     bool
}

// CircuitBreakerConfig configures the circuit breaker.
type CircuitBreakerConfig struct {
	Enabled          bool
	FailureThreshold int
	SuccessThreshold int
	Timeout          time.Duration
}

// DefaultRetryConfig returns the default retry configuration.
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries: 3,
		BaseDelay:  1 * time.Second,
		MaxDelay:   30 * time.Second,
		Factor:     2.0,
		Jitter:     true,
	}
}

// DefaultCircuitBreakerConfig returns the default circuit breaker configuration.
func DefaultCircuitBreakerConfig() CircuitBreakerConfig {
	return CircuitBreakerConfig{
		Enabled:          true,
		FailureThreshold: 5,
		SuccessThreshold: 3,
		Timeout:          30 * time.Second,
	}
}

// NewTransport creates a new Transport with the given configuration.
func NewTransport(cfg Config) *Transport {
	if cfg.UserAgent == "" {
		cfg.UserAgent = version.UserAgent()
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Second
	}

	httpClient := &http.Client{
		Timeout: cfg.Timeout,
	}

	t := &Transport{
		client:           httpClient,
		baseURL:          strings.TrimSuffix(cfg.BaseURL, "/"),
		apiKey:           cfg.APIKey,
		accessToken:      cfg.AccessToken,
		adminKey:         cfg.AdminKey,
		userAgent:        cfg.UserAgent,
		headers:          cfg.Headers,
		logger:           cfg.Logger,
		autoRefreshToken: cfg.AutoRefreshToken,
	}

	// Initialize retry policy. NewRetryPolicy defaults the delay fields
	// (BaseDelay, MaxDelay, Factor) individually, so we must NOT replace the
	// whole struct here: doing so would clobber a caller-provided MaxRetries
	// (e.g. WithRetry(RetryConfig{MaxRetries: 5})) back to the default. The
	// caller-provided MaxRetries is always preserved, including 0 (no retries).
	t.retry = NewRetryPolicy(cfg.Retry)

	// Initialize circuit breaker
	if cfg.CircuitBreaker.Enabled {
		t.circuitBreaker = NewCircuitBreaker(cfg.CircuitBreaker)
	}

	// Initialize token refresher if auto-refresh is enabled and we have an access token
	if cfg.AutoRefreshToken && (cfg.AccessToken != "" || cfg.RefreshToken != "") {
		t.tokenRefresher = NewTokenRefresher(
			cfg.BaseURL,
			cfg.APIKey,
			cfg.RefreshToken,
			cfg.AccessToken,
			cfg.Logger,
		)
	}

	return t
}

// SetAccessToken updates the access token (used for token refresh).
func (t *Transport) SetAccessToken(token string) {
	t.setAccessToken(token)
	if t.tokenRefresher != nil {
		t.tokenRefresher.SetAccessToken(token, 0)
	}
}

// getAccessToken returns the current access token under a read lock.
func (t *Transport) getAccessToken() string {
	t.tokenMu.RLock()
	defer t.tokenMu.RUnlock()
	return t.accessToken
}

// setAccessToken stores the access token under a write lock.
func (t *Transport) setAccessToken(token string) {
	t.tokenMu.Lock()
	t.accessToken = token
	t.tokenMu.Unlock()
}

// SetRefreshToken updates the refresh token.
func (t *Transport) SetRefreshToken(token string) {
	if t.tokenRefresher != nil {
		t.tokenRefresher.SetRefreshToken(token)
	}
}

// Request represents an HTTP request to be made.
type Request struct {
	Method string
	Path   string
	Body   any
	// RawBody, when set, is sent verbatim as the request body (skips JSON marshalling of Body).
	// Useful for webhook ingestion endpoints that require signature verification over the exact bytes.
	RawBody     []byte
	Query       map[string]string
	Headers     map[string]string
	UseAdminKey bool
	Idempotent  bool // If true, can be retried for POST
}

// Response represents an HTTP response.
type Response struct {
	StatusCode int
	Body       []byte
	Headers    http.Header
	RequestID  string
}

// Do executes an HTTP request with retry and circuit breaker logic.
func (t *Transport) Do(ctx context.Context, req *Request) (*Response, error) {
	// Check circuit breaker
	if t.circuitBreaker != nil && !t.circuitBreaker.Allow() {
		return nil, NewCircuitBreakerOpenError()
	}

	// Refresh token proactively if needed
	if t.tokenRefresher != nil && t.autoRefreshToken {
		if err := t.tokenRefresher.RefreshIfNeeded(ctx); err != nil {
			t.log("proactive token refresh failed", "error", err)
		} else if token := t.tokenRefresher.GetAccessToken(); token != "" {
			t.setAccessToken(token)
		}
	}

	var lastErr error
	maxAttempts := t.retry.MaxRetries + 1
	tokenRefreshAttempted := false

retryLoop:
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			// Wait before retry.
			delay := t.nextRetryDelay(attempt, lastErr)
			t.log("retrying request", "attempt", attempt, "delay", delay, "path", req.Path)

			select {
			case <-ctx.Done():
				// Client-side cancellation. Break out so the breaker outcome is
				// still recorded once below (releasing any half-open probe);
				// ctx.Err() is non-retryable, so it is treated as a benign,
				// non-failure outcome rather than tripping the breaker.
				lastErr = ctx.Err()
				break retryLoop
			case <-time.After(delay):
			}
		}

		resp, err := t.doOnce(ctx, req)
		if err == nil {
			// Success - record for circuit breaker
			if t.circuitBreaker != nil {
				t.circuitBreaker.RecordSuccess()
			}
			return resp, nil
		}

		lastErr = err

		// Check for 401 and try to refresh token (only once)
		if IsAuthenticationError(err) && t.tokenRefresher != nil && t.autoRefreshToken && !tokenRefreshAttempted {
			tokenRefreshAttempted = true
			t.log("received 401, attempting token refresh")
			// Use ForceRefresh since we got a 401 (token is definitely invalid)
			if refreshErr := t.tokenRefresher.ForceRefresh(ctx); refreshErr == nil {
				if token := t.tokenRefresher.GetAccessToken(); token != "" {
					t.setAccessToken(token)
					// Retry immediately without counting as a retry attempt
					attempt--
					continue
				}
			} else {
				t.log("token refresh failed", "error", refreshErr)
			}
		}

		// Check if we should retry
		if !t.shouldRetry(req, err, attempt) {
			break retryLoop
		}
	}

	// Record the outcome for the circuit breaker exactly once per Do() call,
	// based on the final error class (not per retry attempt):
	//   - retryable errors (5xx, 429, network, timeout) indicate backend or
	//     transport trouble and count as a breaker failure;
	//   - non-retryable errors are client/validation errors (4xx) where the
	//     backend responded correctly, so they are treated as healthy and do
	//     NOT trip the breaker.
	if t.circuitBreaker != nil {
		if IsRetryable(lastErr) {
			t.circuitBreaker.RecordFailure()
		} else {
			t.circuitBreaker.RecordSuccess()
		}
	}

	return nil, lastErr
}

// doOnce executes a single HTTP request.
func (t *Transport) doOnce(ctx context.Context, req *Request) (*Response, error) {
	// Build URL
	fullURL := t.baseURL + req.Path
	if len(req.Query) > 0 {
		// Properly URL-encode query parameters (important for commas, unicode, spaces, etc.)
		q := url.Values{}
		for k, v := range req.Query {
			q.Set(k, v)
		}
		encoded := q.Encode()
		if encoded != "" {
			fullURL += "?" + encoded
		}
	}

	// Prepare body
	var bodyReader io.Reader
	if req.RawBody != nil {
		bodyReader = bytes.NewReader(req.RawBody)
	} else if req.Body != nil {
		bodyBytes, err := json.Marshal(req.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(bodyBytes)
	}

	// Create HTTP request
	httpReq, err := http.NewRequestWithContext(ctx, req.Method, fullURL, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	httpReq.Header.Set("User-Agent", t.userAgent)
	httpReq.Header.Set("Accept", "application/json")
	if req.Body != nil || req.RawBody != nil {
		httpReq.Header.Set("Content-Type", "application/json")
	}

	// Set auth header. Read the access token once under the lock; it may be
	// mutated concurrently by token refresh in other goroutines.
	accessToken := t.getAccessToken()
	if req.UseAdminKey && t.adminKey != "" {
		httpReq.Header.Set("X-Admin-Key", t.adminKey)
	} else if accessToken != "" {
		httpReq.Header.Set("Authorization", "Bearer "+accessToken)
	} else if t.apiKey != "" {
		// API keys are sent via Bearer token, not X-API-Key header
		httpReq.Header.Set("Authorization", "Bearer "+t.apiKey)
	}

	// Add custom headers from transport config
	for k, v := range t.headers {
		httpReq.Header.Set(k, v)
	}

	// Add request-specific headers
	for k, v := range req.Headers {
		httpReq.Header.Set(k, v)
	}

	// Execute request
	t.log("executing request", "method", req.Method, "url", fullURL)
	httpResp, err := t.client.Do(httpReq)
	if err != nil {
		// Check for timeout
		if ctx.Err() != nil {
			return nil, NewTimeoutError(t.client.Timeout, ctx.Err())
		}
		return nil, NewNetworkError(err)
	}
	defer httpResp.Body.Close()

	// Read body
	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, NewNetworkError(fmt.Errorf("failed to read response body: %w", err))
	}

	resp := &Response{
		StatusCode: httpResp.StatusCode,
		Body:       body,
		Headers:    httpResp.Header,
		RequestID:  httpResp.Header.Get("X-Request-ID"),
	}

	t.log("received response", "status", resp.StatusCode, "request_id", resp.RequestID)

	// Check for errors
	if httpResp.StatusCode >= 400 {
		return nil, ParseErrorFromResponse(httpResp.StatusCode, body, httpResp.Header)
	}

	return resp, nil
}

// nextRetryDelay computes the wait before the given retry attempt (1-based).
// It uses exponential backoff, but honors a server-provided Retry-After on 429
// responses: the wait is never shorter than the server asked for, never
// shorter than the exponential backoff, and never longer than the configured
// MaxDelay.
func (t *Transport) nextRetryDelay(attempt int, lastErr error) time.Duration {
	delay := t.retry.Delay(attempt - 1)

	var rateLimitErr *RateLimitError
	if errors.As(lastErr, &rateLimitErr) && rateLimitErr.RetryAfter > 0 {
		if rateLimitErr.RetryAfter > delay {
			delay = rateLimitErr.RetryAfter
		}
		if delay > t.retry.MaxDelay {
			delay = t.retry.MaxDelay
		}
	}
	return delay
}

// shouldRetry determines if a request should be retried.
func (t *Transport) shouldRetry(req *Request, err error, attempt int) bool {
	if attempt >= t.retry.MaxRetries {
		return false
	}

	// Don't retry non-idempotent requests unless explicitly marked
	if req.Method == http.MethodPost && !req.Idempotent {
		return false
	}

	// Retry based on error type
	return IsRetryable(err)
}

// log logs a debug message.
func (t *Transport) log(msg string, keysAndValues ...any) {
	if t.logger != nil {
		t.logger.Debug(msg, keysAndValues...)
	}
}

// JSON decodes a response body into a target value.
func JSON[T any](resp *Response) (*T, error) {
	if len(resp.Body) == 0 {
		return nil, nil
	}
	var result T
	if err := json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}
	return &result, nil
}

// JSONArray decodes a response body into a slice.
func JSONArray[T any](resp *Response) ([]T, error) {
	if len(resp.Body) == 0 {
		return nil, nil
	}
	var result []T
	if err := json.Unmarshal(resp.Body, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}
	return result, nil
}
