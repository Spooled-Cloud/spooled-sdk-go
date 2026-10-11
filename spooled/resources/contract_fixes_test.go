package resources

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/spooled-cloud/spooled-sdk-go/internal/httpx"
)

// newRetryingTransport is a transport with fast retries and the breaker on, the
// shape a real client has (the default config retries 3 times).
func newRetryingTransport(url string) *httpx.Transport {
	return httpx.NewTransport(httpx.Config{
		BaseURL: url,
		APIKey:  "sp_test_key",
		Retry:   httpx.RetryConfig{MaxRetries: 3, BaseDelay: time.Millisecond, MaxDelay: 5 * time.Millisecond, Factor: 2},
		CircuitBreaker: httpx.CircuitBreakerConfig{
			Enabled: true, FailureThreshold: 2, SuccessThreshold: 1, Timeout: time.Minute,
		},
	})
}

// --- SDKGP-01: avg_*_ms are fractional ---------------------------------------

func TestQueuesGetStats_AcceptsFractionalAverage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"queue_name":"emails","pending_jobs":3,"processing_jobs":1,"completed_jobs_24h":10,` +
			`"failed_jobs_24h":2,"avg_processing_time_ms":1523.456,"max_job_age_seconds":12,"active_workers":2}`))
	}))
	defer server.Close()

	res := NewQueuesResource(httpx.NewTransport(httpx.Config{BaseURL: server.URL, APIKey: "sp_test_key"}))
	got, err := res.GetStats(context.Background(), "emails")
	if err != nil {
		t.Fatalf("GetStats failed for a queue with completed jobs: %v", err)
	}
	if got.AvgProcessingTimeMs == nil || *got.AvgProcessingTimeMs != 1523.456 {
		t.Errorf("AvgProcessingTimeMs = %v, want 1523.456", got.AvgProcessingTimeMs)
	}
	if got.PendingJobs != 3 || got.ActiveWorkers != 2 || got.MaxJobAgeSeconds == nil || *got.MaxJobAgeSeconds != 12 {
		t.Errorf("other fields lost: %+v", got)
	}
}

func TestDashboardGet_AcceptsFractionalAverages(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"system":{"version":"0.1.0","uptime_seconds":5},"jobs":{"total":4,"avg_wait_time_ms":12.5,` +
			`"avg_processing_time_ms":1523.456},"queues":[],"workers":{},"recent_activity":{}}`))
	}))
	defer server.Close()

	res := NewDashboardResource(httpx.NewTransport(httpx.Config{BaseURL: server.URL, APIKey: "sp_test_key"}))
	got, err := res.Get(context.Background())
	if err != nil {
		t.Fatalf("Dashboard.Get: %v", err)
	}
	if got.Jobs.AvgWaitTimeMs == nil || *got.Jobs.AvgWaitTimeMs != 12.5 {
		t.Errorf("AvgWaitTimeMs = %v, want 12.5", got.Jobs.AvgWaitTimeMs)
	}
}

// --- SDKGP-16: integers above 2^53 -------------------------------------------

func TestDecode_LargeIntegersSurviveAndOrdinaryNumbersStayFloat64(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"jobs":[{"id":"j1","queue_name":"q","retry_count":0,"max_retries":3,"timeout_seconds":30,` +
			`"payload":{"discord_id":1234567890123456789,"edge":9007199254740993,"small":42,"ratio":0.25,` +
			`"neg":-9007199254740993,"nested":{"ids":[1234567890123456789,7]}}}]}`))
	}))
	defer server.Close()

	res := NewJobsResource(httpx.NewTransport(httpx.Config{BaseURL: server.URL, APIKey: "sp_test_key"}))
	claimed, err := res.Claim(context.Background(), &ClaimJobsRequest{QueueName: "q", WorkerID: "w"})
	if err != nil {
		t.Fatalf("Claim: %v", err)
	}
	payload, ok := claimed.Jobs[0].Payload.(map[string]any)
	if !ok {
		t.Fatalf("payload = %T, want an object", claimed.Jobs[0].Payload)
	}

	// Beyond float64's exact range: kept exact as json.Number.
	for key, want := range map[string]string{
		"discord_id": "1234567890123456789",
		"edge":       "9007199254740993",
		"neg":        "-9007199254740993",
	} {
		n, isNumber := payload[key].(json.Number)
		if !isNumber || n.String() != want {
			t.Errorf("payload[%s] = %#v, want json.Number(%s)", key, payload[key], want)
		}
	}
	if id, err := payload["discord_id"].(json.Number).Int64(); err != nil || id != 1234567890123456789 {
		t.Errorf("Int64() = %d, %v", id, err)
	}
	nested := payload["nested"].(map[string]any)["ids"].([]any)
	if n, isNumber := nested[0].(json.Number); !isNumber || n.String() != "1234567890123456789" {
		t.Errorf("nested id = %#v, want json.Number", nested[0])
	}

	// Everything float64 can hold stays a float64, so existing .(float64)
	// assertions keep working.
	if v, isFloat := payload["small"].(float64); !isFloat || v != 42 {
		t.Errorf("payload[small] = %#v, want float64(42)", payload["small"])
	}
	if v, isFloat := payload["ratio"].(float64); !isFloat || v != 0.25 {
		t.Errorf("payload[ratio] = %#v, want float64(0.25)", payload["ratio"])
	}
	if v, isFloat := nested[1].(float64); !isFloat || v != 7 {
		t.Errorf("nested[1] = %#v, want float64(7)", nested[1])
	}
}

func TestDecode_JobGetKeepsLargeIDsAndLenientWholeFloats(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// retry_count arrives as a whole-number float: it decoded into an int
		// before and must keep doing so.
		_, _ = w.Write([]byte(`{"id":"j1","organization_id":"o","queue_name":"q","status":"completed","retry_count":2.0,` +
			`"max_retries":3,"priority":0,"timeout_seconds":300,"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z",` +
			`"payload":{"id":1234567890123456789},"result":{"count":3,"id":9007199254740993}}`))
	}))
	defer server.Close()

	res := NewJobsResource(httpx.NewTransport(httpx.Config{BaseURL: server.URL, APIKey: "sp_test_key"}))
	job, err := res.Get(context.Background(), "j1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if job.RetryCount != 2 {
		t.Errorf("RetryCount = %d, want 2", job.RetryCount)
	}
	if n, ok := job.Payload.(map[string]any)["id"].(json.Number); !ok || n.String() != "1234567890123456789" {
		t.Errorf("payload id = %#v", job.Payload)
	}
	result := job.Result.(map[string]any)
	if n, ok := result["id"].(json.Number); !ok || n.String() != "9007199254740993" {
		t.Errorf("result id = %#v", result["id"])
	}
	if v, ok := result["count"].(float64); !ok || v != 3 {
		t.Errorf("result count = %#v, want float64(3)", result["count"])
	}

	// Direct json.Unmarshal of a Job keeps the same contract.
	var direct Job
	if err := json.Unmarshal([]byte(`{"id":"j","payload":{"a":5,"b":1234567890123456789}}`), &direct); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	p := direct.Payload.(map[string]any)
	if _, ok := p["a"].(float64); !ok {
		t.Errorf("direct payload a = %#v, want float64", p["a"])
	}
	if _, ok := p["b"].(json.Number); !ok {
		t.Errorf("direct payload b = %#v, want json.Number", p["b"])
	}
}

// --- Item 2: acks are retried -------------------------------------------------

func TestAcks_RetryTransientFailuresAndIgnoreOpenBreaker(t *testing.T) {
	var completeCalls, failCalls, heartbeatCalls, workerHeartbeatCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/boom"):
			w.WriteHeader(http.StatusInternalServerError)
		case strings.HasSuffix(r.URL.Path, "/complete"):
			if completeCalls.Add(1) < 3 { // two 503s, then success
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			_, _ = w.Write([]byte(`{"success":true}`))
		case strings.HasSuffix(r.URL.Path, "/fail"):
			if failCalls.Add(1) < 2 {
				w.WriteHeader(http.StatusBadGateway)
				return
			}
			_, _ = w.Write([]byte(`{"success":true,"will_retry":true}`))
		case strings.HasSuffix(r.URL.Path, "/heartbeat") && strings.Contains(r.URL.Path, "/workers/"):
			if workerHeartbeatCalls.Add(1) < 2 {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			_, _ = w.Write([]byte(`{}`))
		case strings.HasSuffix(r.URL.Path, "/heartbeat"):
			if heartbeatCalls.Add(1) < 2 {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			_, _ = w.Write([]byte(`{"success":true}`))
		}
	}))
	defer server.Close()

	transport := newRetryingTransport(server.URL)
	jobs := NewJobsResource(transport)
	workers := NewWorkersResource(transport)
	ctx := context.Background()

	// Trip the breaker with unrelated traffic: acks must still go through.
	for i := 0; i < 3; i++ {
		_ = NewBase(transport).Get(ctx, "/api/v1/boom", nil)
	}
	if err := NewBase(transport).Get(ctx, "/api/v1/anything", nil); err == nil || !strings.Contains(err.Error(), "circuit breaker") {
		t.Fatalf("test setup: breaker should be open, got %v", err)
	}

	if err := jobs.Complete(ctx, "j1", &CompleteJobRequest{WorkerID: "w"}); err != nil {
		t.Fatalf("Complete should survive two 503s: %v", err)
	}
	if got := completeCalls.Load(); got != 3 {
		t.Errorf("complete attempts = %d, want 3", got)
	}
	if _, err := jobs.FailWithResponse(ctx, "j1", &FailJobRequest{WorkerID: "w", Error: "x"}); err != nil {
		t.Fatalf("Fail should survive a 502: %v", err)
	}
	if err := jobs.Heartbeat(ctx, "j1", &HeartbeatRequest{WorkerID: "w"}); err != nil {
		t.Fatalf("Heartbeat should survive a 503: %v", err)
	}
	if _, err := jobs.RenewLease(ctx, "j1", &RenewLeaseRequest{WorkerID: "w", LeaseDurationSec: 30}); err != nil {
		t.Fatalf("RenewLease: %v", err)
	}
	if err := workers.Heartbeat(ctx, "w1", &WorkerHeartbeatRequest{CurrentJobs: 1}); err != nil {
		t.Fatalf("worker Heartbeat should survive a 503: %v", err)
	}
}

func TestAcks_DoNotRetryClientErrors(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"code":"LEASE_EXPIRED","message":"lease expired"}`))
	}))
	defer server.Close()

	jobs := NewJobsResource(newRetryingTransport(server.URL))
	if err := jobs.Complete(context.Background(), "j1", &CompleteJobRequest{WorkerID: "w"}); err == nil {
		t.Fatal("expected the 409")
	}
	if got := calls.Load(); got != 1 {
		t.Errorf("a 409 was attempted %d times, want 1", got)
	}
}

// --- Item 3: quota 429 -------------------------------------------------------

func TestEnqueueQuotaExceeded_FailsFastAndLeavesAcksWorking(t *testing.T) {
	var createCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/jobs" {
			createCalls.Add(1)
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":"limit_exceeded","code":"QUOTA_EXCEEDED","message":"full","resource":"active_jobs",` +
				`"current":100,"limit":100,"plan":"free"}`))
			return
		}
		_, _ = w.Write([]byte(`{"success":true}`))
	}))
	defer server.Close()

	transport := httpx.NewTransport(httpx.Config{
		BaseURL: server.URL,
		APIKey:  "sp_test_key",
		// A one-hour backoff: if the quota were retried the test would hang.
		Retry:          httpx.RetryConfig{MaxRetries: 3, BaseDelay: time.Hour, MaxDelay: time.Hour, Factor: 2},
		CircuitBreaker: httpx.CircuitBreakerConfig{Enabled: true, FailureThreshold: 2, SuccessThreshold: 1, Timeout: time.Minute},
	})
	jobs := NewJobsResource(transport)
	ctx := context.Background()

	for i := 0; i < 4; i++ {
		_, err := jobs.Create(ctx, &CreateJobRequest{QueueName: "q", Payload: map[string]any{}})
		var rl *httpx.RateLimitError
		if !errors.As(err, &rl) || !rl.IsQuotaExceeded() || rl.Quota.Plan != "free" {
			t.Fatalf("attempt %d: err = %v, want a quota RateLimitError", i, err)
		}
	}
	if got := createCalls.Load(); got != 4 {
		t.Errorf("server saw %d creates for 4 calls, want 4 (no retries)", got)
	}
	if err := jobs.Complete(ctx, "j1", &CompleteJobRequest{WorkerID: "w"}); err != nil {
		t.Errorf("Complete was blocked after enqueue quota errors: %v", err)
	}
	if _, err := jobs.Get(ctx, "j1"); err != nil {
		t.Errorf("breaker tripped by quota errors: %v", err)
	}
}

// --- Item 4 / SDKGP-12: failure text -----------------------------------------

func TestCoerceFailureMessage(t *testing.T) {
	long := strings.Repeat("a", 5000)
	multibyte := strings.Repeat("日", 3000) // 3 bytes per character
	tests := []struct {
		name string
		in   string
		want func(string) bool
	}{
		{"empty becomes default", "", func(s string) bool { return s == "Job failed" }},
		{"blank becomes default", "  \n\t", func(s string) bool { return s == "Job failed" }},
		{"short is untouched", "boom", func(s string) bool { return s == "boom" }},
		{"exactly 2048 is untouched", strings.Repeat("a", 2048), func(s string) bool { return len(s) == 2048 }},
		{"long ascii is cut to 2048", long, func(s string) bool { return len(s) == 2048 }},
		{"long multibyte is cut on a character boundary", multibyte, func(s string) bool {
			return len([]rune(s)) == 2048 && strings.ToValidUTF8(s, "") == s && strings.HasSuffix(s, "日")
		}},
		{"2048 multibyte characters are untouched", strings.Repeat("日", 2048), func(s string) bool { return len([]rune(s)) == 2048 }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CoerceFailureMessage(tt.in); !tt.want(got) {
				t.Errorf("CoerceFailureMessage(%.20q...) = %.20q... (len %d)", tt.in, got, len(got))
			}
		})
	}
}

func TestFail_SendsAcceptableErrorText(t *testing.T) {
	var bodies []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		bodies = append(bodies, body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true}`))
	}))
	defer server.Close()

	jobs := NewJobsResource(httpx.NewTransport(httpx.Config{BaseURL: server.URL, APIKey: "sp_test_key"}))
	req := &FailJobRequest{WorkerID: "w", Error: ""}
	if err := jobs.Fail(context.Background(), "j1", req); err != nil {
		t.Fatalf("Fail: %v", err)
	}
	if err := jobs.Fail(context.Background(), "j1", &FailJobRequest{WorkerID: "w", Error: strings.Repeat("x", 5000)}); err != nil {
		t.Fatalf("Fail: %v", err)
	}
	if got := bodies[0]["error"]; got != "Job failed" {
		t.Errorf("empty error sent as %v", got)
	}
	if got := bodies[1]["error"].(string); len(got) != 2048 {
		t.Errorf("long error sent with %d characters, want 2048", len(got))
	}
	if req.Error != "" {
		t.Errorf("the caller's request was mutated: %q", req.Error)
	}
}

// --- SDKGP-10 / SDKGP-13: unsupported methods make no network call ------------

func TestUnsupportedMethods_FailWithTypedErrorWithoutNetwork(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()
	transport := httpx.NewTransport(httpx.Config{BaseURL: server.URL, APIKey: "sp_test_key"})
	ctx := context.Background()

	checks := map[string]error{
		"Jobs.UpdateProgress": NewJobsResource(transport).UpdateProgress(ctx, "j1", &UpdateProgressRequest{Progress: 50}),
		"Ingest.GitHub":       NewIngestResource(transport).GitHub(ctx, "org", []byte(`{}`), &GitHubIngestOptions{GitHubEvent: "push", Secret: "s"}),
		"Ingest.Stripe":       NewIngestResource(transport).Stripe(ctx, "org", []byte(`{}`), &StripeIngestOptions{Secret: "s"}),
	}
	for op, err := range checks {
		var unsupported *UnsupportedError
		if !errors.Is(err, ErrUnsupported) || !errors.As(err, &unsupported) || unsupported.Operation != op {
			t.Errorf("%s: err = %v, want an UnsupportedError for it", op, err)
		}
		if err != nil && !strings.Contains(err.Error(), "not supported by the Spooled API") {
			t.Errorf("%s: message %q does not explain itself", op, err.Error())
		}
	}
	if got := calls.Load(); got != 0 {
		t.Errorf("unsupported methods made %d network calls, want 0", got)
	}
}

func TestIngestCustom_RequiresWebhookTokenAndSendsHeader(t *testing.T) {
	var calls atomic.Int32
	var gotToken, gotBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		gotToken = r.Header.Get("X-Webhook-Token")
		raw, _ := io.ReadAll(r.Body)
		gotBody = string(raw)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"job_id":"j1","queue_name":"events","status":"pending"}`))
	}))
	defer server.Close()
	res := NewIngestResource(httpx.NewTransport(httpx.Config{BaseURL: server.URL, APIKey: "sp_test_key"}))

	_, err := res.Custom(context.Background(), "org_1", &CustomWebhookRequest{QueueName: "events", Payload: map[string]any{"a": 1}})
	if !errors.Is(err, ErrWebhookTokenRequired) {
		t.Fatalf("Custom without a token err = %v, want ErrWebhookTokenRequired", err)
	}
	if calls.Load() != 0 {
		t.Error("Custom without a token still made a request (the API would answer 401)")
	}

	if _, err := res.Custom(context.Background(), "org_1", &CustomWebhookRequest{
		QueueName: "events", Payload: map[string]any{"a": 1}, WebhookToken: "whk_secret",
	}); err != nil {
		t.Fatalf("Custom: %v", err)
	}
	if gotToken != "whk_secret" {
		t.Errorf("X-Webhook-Token = %q, want whk_secret", gotToken)
	}
	if strings.Contains(gotBody, "whk_secret") || strings.Contains(gotBody, "WebhookToken") {
		t.Errorf("the token leaked into the request body: %s", gotBody)
	}
}

// --- SDKGP-14: list helpers only send what the API honors -------------------

func TestSchedulesList_SendsOnlyLimitAndFiltersClientSide(t *testing.T) {
	var gotQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{"id":"s1","name":"a","queue_name":"mail","is_active":true},
			{"id":"s2","name":"b","queue_name":"mail","is_active":false},
			{"id":"s3","name":"c","queue_name":"sms","is_active":true},
			{"id":"s4","name":"d","queue_name":"mail","is_active":true}
		]`))
	}))
	defer server.Close()
	res := NewSchedulesResource(httpx.NewTransport(httpx.Config{BaseURL: server.URL, APIKey: "sp_test_key"}))
	ctx := context.Background()
	mail, active, one, zero := "mail", true, 1, 0

	all, err := res.List(ctx, nil)
	if err != nil || len(all) != 4 || gotQuery != "" {
		t.Fatalf("plain List = %d schedules, query %q, err %v", len(all), gotQuery, err)
	}

	only, err := res.List(ctx, &ListSchedulesParams{QueueName: &mail, IsActive: &active})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if gotQuery != "limit=500" {
		t.Errorf("query = %q, want only limit=500 (queue_name/is_active are ignored by the API)", gotQuery)
	}
	if len(only) != 2 || only[0].ID != "s1" || only[1].ID != "s4" {
		t.Errorf("filtered = %+v, want s1 and s4", only)
	}

	// Offset pages client-side instead of re-reading the first page forever.
	page2, _ := res.List(ctx, &ListSchedulesParams{Offset: &one, Limit: &one})
	if len(page2) != 1 || page2[0].ID != "s2" {
		t.Errorf("offset page = %+v, want s2", page2)
	}
	empty, _ := res.List(ctx, &ListSchedulesParams{Offset: &zero, Limit: &zero})
	if len(empty) != 0 {
		t.Errorf("limit 0 returned %d schedules", len(empty))
	}

	if _, err := res.List(ctx, &ListSchedulesParams{Limit: &one}); err != nil || gotQuery != "limit=1" {
		t.Errorf("Limit alone: query %q err %v, want limit=1", gotQuery, err)
	}
}

func TestWebhookDeliveries_SendsNoQueryAndFiltersClientSide(t *testing.T) {
	var gotQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{"id":"d1","webhook_id":"w","status":"failed","attempts":1,"created_at":"2026-01-01T00:00:00Z"},
			{"id":"d2","webhook_id":"w","status":"success","attempts":1,"created_at":"2026-01-01T00:00:00Z"},
			{"id":"d3","webhook_id":"w","status":"failed","attempts":2,"created_at":"2026-01-01T00:00:00Z"}
		]`))
	}))
	defer server.Close()
	res := NewWebhooksResource(httpx.NewTransport(httpx.Config{BaseURL: server.URL, APIKey: "sp_test_key"}))
	failed, one := WebhookDeliveryStatusFailed, 1

	got, err := res.Deliveries(context.Background(), "w", &ListDeliveriesParams{Status: &failed, Offset: &one})
	if err != nil {
		t.Fatalf("Deliveries: %v", err)
	}
	if gotQuery != "" {
		t.Errorf("query = %q, want none (the API takes no parameters)", gotQuery)
	}
	if len(got) != 1 || got[0].ID != "d3" {
		t.Errorf("deliveries = %+v, want only d3 (second failed delivery)", got)
	}
}

// --- P3: probes answer 200 with an empty body --------------------------------

func TestHealthProbes_EmptyBodyYieldsAStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK) // the API sends no body from the probes
	}))
	defer server.Close()
	res := NewHealthResource(httpx.NewTransport(httpx.Config{BaseURL: server.URL, APIKey: "sp_test_key"}))

	live, err := res.Liveness(context.Background())
	if err != nil || live.Status != "alive" {
		t.Errorf("Liveness = %+v, %v; want Status alive", live, err)
	}
	ready, err := res.Readiness(context.Background())
	if err != nil || ready.Status != "ready" {
		t.Errorf("Readiness = %+v, %v; want Status ready", ready, err)
	}
}
