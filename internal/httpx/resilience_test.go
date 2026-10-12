package httpx

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// quotaBody is what the API sends for an over-quota request: 429, no
// Retry-After, code QUOTA_EXCEEDED plus the plan-limit details.
const quotaBody = `{"error":"limit_exceeded","code":"QUOTA_EXCEEDED","message":"You have reached the limit",` +
	`"resource":"active_jobs","current":100,"limit":100,"plan":"free","upgrade_to":"starter"}`

func fastRetry(maxRetries int) RetryConfig {
	return RetryConfig{MaxRetries: maxRetries, BaseDelay: time.Millisecond, MaxDelay: 5 * time.Millisecond, Factor: 2}
}

// TestRetryPolicy_DelayIsSafeForConcurrentUse is the -race regression test for
// SDKGP-15: one policy is shared by every goroutine using the client, and its
// per-policy *rand.Rand was used without a lock.
func TestRetryPolicy_DelayIsSafeForConcurrentUse(t *testing.T) {
	p := NewRetryPolicy(RetryConfig{BaseDelay: time.Millisecond, MaxDelay: time.Second, Factor: 2, Jitter: true})

	var wg sync.WaitGroup
	for g := 0; g < 16; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 500; i++ {
				if d := p.Delay(i % 5); d <= 0 {
					t.Errorf("Delay = %v, want positive", d)
					return
				}
			}
		}()
	}
	wg.Wait()
}

func TestParseErrorFromResponse_QuotaExceededIsNotRetryable(t *testing.T) {
	err := ParseErrorFromResponse(http.StatusTooManyRequests, []byte(quotaBody), http.Header{})

	var rl *RateLimitError
	if !errors.As(err, &rl) {
		t.Fatalf("err = %T, want *RateLimitError (callers match IsRateLimitError on every 429)", err)
	}
	if !rl.IsQuotaExceeded() || rl.Quota == nil {
		t.Fatalf("QUOTA_EXCEEDED 429 not classified as a quota: %+v", rl)
	}
	if rl.Quota.Resource != "active_jobs" || rl.Quota.Current != 100 || rl.Quota.Limit != 100 ||
		rl.Quota.Plan != "free" || rl.Quota.UpgradeTo != "starter" {
		t.Errorf("Quota = %+v, want the plan-limit details from the body", rl.Quota)
	}
	if IsRetryable(err) {
		t.Error("a plan quota must not be retryable")
	}
}

func TestParseErrorFromResponse_PlanLimitShapeWithoutCodeIsQuota(t *testing.T) {
	body := `{"error":"limit","message":"full","resource":"queues","current":5,"limit":5,"plan":"free"}`
	err := ParseErrorFromResponse(http.StatusTooManyRequests, []byte(body), http.Header{})
	if IsRetryable(err) {
		t.Error("a 429 without Retry-After that carries a plan-limit body must not be retried")
	}
}

func TestParseErrorFromResponse_RateLimitWithRetryAfterStaysRetryable(t *testing.T) {
	h := http.Header{}
	h.Set("Retry-After", "2")
	err := ParseErrorFromResponse(http.StatusTooManyRequests,
		[]byte(`{"code":"RATE_LIMIT_EXCEEDED","message":"slow down"}`), h)

	var rl *RateLimitError
	if !errors.As(err, &rl) || rl.IsQuotaExceeded() {
		t.Fatalf("plain rate limit misclassified: %#v", err)
	}
	if !IsRetryable(err) {
		t.Error("a rate limit with Retry-After must stay retryable")
	}
}

func TestAPIError_IsRetryable408(t *testing.T) {
	if !ParseErrorFromResponse(http.StatusRequestTimeout, nil, http.Header{}).(retryableErr).IsRetryable() {
		t.Error("408 must be retryable")
	}
	if ParseErrorFromResponse(http.StatusBadRequest, nil, http.Header{}).(retryableErr).IsRetryable() {
		t.Error("400 must not be retryable")
	}
}

type retryableErr interface{ IsRetryable() bool }

// TestTransport_QuotaExceeded_NotRetriedAndDoesNotTripBreaker covers the Go side
// of "429 QUOTA_EXCEEDED is not transient": one request despite MaxRetries, no
// long sleeps, and the shared breaker stays closed so acks keep flowing.
func TestTransport_QuotaExceeded_NotRetriedAndDoesNotTripBreaker(t *testing.T) {
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/quota" {
			hits.Add(1)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(quotaBody))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	transport := NewTransport(Config{
		BaseURL:        server.URL,
		APIKey:         "sp_test_000000000000000000000000000000",
		Retry:          RetryConfig{MaxRetries: 3, BaseDelay: time.Hour, MaxDelay: time.Hour, Factor: 2},
		CircuitBreaker: CircuitBreakerConfig{Enabled: true, FailureThreshold: 2, SuccessThreshold: 1, Timeout: time.Minute},
	})

	start := time.Now()
	for i := 0; i < 5; i++ {
		_, err := transport.Do(context.Background(), &Request{Method: http.MethodGet, Path: "/quota"})
		if !IsRateLimitError(err) {
			t.Fatalf("attempt %d: err = %v, want a rate-limit-typed quota error", i, err)
		}
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("quota errors slept %v; they must fail fast", elapsed)
	}
	if got := hits.Load(); got != 5 {
		t.Errorf("server saw %d requests for 5 calls, want 5 (no retries)", got)
	}
	if st := transport.circuitBreaker.State(); st != CircuitClosed {
		t.Errorf("breaker state = %v after quota errors, want closed", st)
	}
	if _, err := transport.Do(context.Background(), &Request{Method: http.MethodGet, Path: "/ok"}); err != nil {
		t.Errorf("an unrelated request was blocked after quota errors: %v", err)
	}
}

func TestTransport_SkipCircuitBreaker_SendsWhileOpen(t *testing.T) {
	var ackHits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/ack" {
			ackHits.Add(1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"success":true}`))
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	transport := NewTransport(Config{
		BaseURL:        server.URL,
		APIKey:         "sp_test_000000000000000000000000000000",
		Retry:          fastRetry(0),
		CircuitBreaker: CircuitBreakerConfig{Enabled: true, FailureThreshold: 1, SuccessThreshold: 1, Timeout: time.Minute},
	})

	_, _ = transport.Do(context.Background(), &Request{Method: http.MethodGet, Path: "/boom"})
	if st := transport.circuitBreaker.State(); st != CircuitOpen {
		t.Fatalf("breaker state = %v, want open", st)
	}

	// A normal request is refused...
	if _, err := transport.Do(context.Background(), &Request{Method: http.MethodPost, Path: "/ack"}); err == nil {
		t.Fatal("expected the open breaker to refuse a normal request")
	}
	// ...but an ack that opts out goes through.
	if _, err := transport.Do(context.Background(), &Request{Method: http.MethodPost, Path: "/ack", SkipCircuitBreaker: true}); err != nil {
		t.Fatalf("ack with SkipCircuitBreaker failed: %v", err)
	}
	if got := ackHits.Load(); got != 1 {
		t.Errorf("ack reached the server %d times, want 1", got)
	}
	if st := transport.circuitBreaker.State(); st != CircuitOpen {
		t.Errorf("a bypassed request changed the breaker state to %v", st)
	}
}

func TestTransport_CallerCancellationDoesNotTripBreaker(t *testing.T) {
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	defer server.Close()
	defer close(release)

	transport := NewTransport(Config{
		BaseURL:        server.URL,
		APIKey:         "sp_test_000000000000000000000000000000",
		Retry:          fastRetry(0),
		CircuitBreaker: CircuitBreakerConfig{Enabled: true, FailureThreshold: 1, SuccessThreshold: 1, Timeout: time.Minute},
	})

	for i := 0; i < 3; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		go func() {
			time.Sleep(20 * time.Millisecond)
			cancel()
		}()
		_, err := transport.Do(ctx, &Request{Method: http.MethodPost, Path: "/claim"})
		if err == nil {
			t.Fatal("expected a cancellation error")
		}
	}
	if st := transport.circuitBreaker.State(); st != CircuitClosed {
		t.Errorf("breaker state = %v after caller-canceled requests, want closed", st)
	}
}

// TestTransport_IdempotentPOSTRetriesNetworkErrorsAnd5xxAnd408 is the transport
// half of "acks are retried": complete/fail/heartbeat are sent as idempotent
// POSTs, so a 503, a 408 and a dropped connection are retried.
func TestTransport_IdempotentPOSTRetriesTransientFailures(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		switch n {
		case 1:
			w.WriteHeader(http.StatusServiceUnavailable)
		case 2:
			w.WriteHeader(http.StatusRequestTimeout)
		case 3:
			// Drop the connection without a response: a network error.
			hj, _ := w.(http.Hijacker)
			conn, _, _ := hj.Hijack()
			_ = conn.Close()
		default:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"success":true}`))
		}
	}))
	defer server.Close()

	transport := NewTransport(Config{BaseURL: server.URL, APIKey: "sp_test_000000000000000000000000000000", Retry: fastRetry(3)})
	if _, err := transport.Do(context.Background(), &Request{Method: http.MethodPost, Path: "/ack", Idempotent: true}); err != nil {
		t.Fatalf("idempotent POST should survive 503, 408 and a dropped connection: %v", err)
	}
	if got := calls.Load(); got != 4 {
		t.Errorf("server saw %d attempts, want 4", got)
	}
}

func TestTransport_IdempotentPOSTDoesNotRetry4xx(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"code":"LEASE_EXPIRED","message":"stale"}`))
	}))
	defer server.Close()

	transport := NewTransport(Config{BaseURL: server.URL, APIKey: "sp_test_000000000000000000000000000000", Retry: fastRetry(3)})
	if _, err := transport.Do(context.Background(), &Request{Method: http.MethodPost, Path: "/ack", Idempotent: true}); err == nil {
		t.Fatal("expected the 409 to surface")
	}
	if got := calls.Load(); got != 1 {
		t.Errorf("a 409 was retried: %d attempts, want 1", got)
	}
}
