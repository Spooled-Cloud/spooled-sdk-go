package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/spooled-cloud/spooled-sdk-go/internal/httpx"
	"github.com/spooled-cloud/spooled-sdk-go/spooled/resources"
)

// apiCall is one request the fake API received.
type apiCall struct {
	Path string
	Body map[string]any
}

// fakeAPI is an ordered, programmable Spooled REST backend. It serves one batch
// of claimed jobs, then empty claims, and records every request in order so
// tests can assert what was sent and in which order (for example that the worker
// deregisters only after its acks).
type fakeAPI struct {
	t      *testing.T
	server *httptest.Server

	mu        sync.Mutex
	claimJobs string
	served    bool
	registers int
	calls     []apiCall

	// respond, when set, may answer a request itself. Return handled=false to fall
	// through to the default answer.
	respond func(path string, nth int, w http.ResponseWriter) (handled bool)
	counts  map[string]int
}

func newFakeAPI(t *testing.T, claimJobs string) *fakeAPI {
	t.Helper()
	f := &fakeAPI{t: t, claimJobs: claimJobs, counts: map[string]int{}}
	f.server = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.server.Close)
	return f
}

func (f *fakeAPI) serve(w http.ResponseWriter, r *http.Request) {
	raw, _ := io.ReadAll(r.Body)
	var body map[string]any
	_ = json.Unmarshal(raw, &body)
	path := r.URL.Path

	f.mu.Lock()
	f.calls = append(f.calls, apiCall{Path: path, Body: body})
	f.counts[path]++
	nth := f.counts[path]
	respond := f.respond
	f.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	if respond != nil && respond(path, nth, w) {
		return
	}

	switch {
	case path == "/api/v1/workers/register":
		f.mu.Lock()
		f.registers++
		f.mu.Unlock()
		_, _ = w.Write([]byte(`{"id":"w-test","queue_name":"q","lease_duration_secs":30,"heartbeat_interval_secs":15}`))
	case path == "/api/v1/jobs/claim":
		f.mu.Lock()
		jobs := f.claimJobs
		if f.served {
			jobs = `{"jobs":[]}`
		}
		f.served = true
		f.mu.Unlock()
		_, _ = w.Write([]byte(jobs))
	case strings.HasSuffix(path, "/fail"):
		_, _ = w.Write([]byte(`{"success":true,"will_retry":true}`))
	default:
		_, _ = w.Write([]byte(`{"success":true}`))
	}
}

// to returns, in order, the calls whose path ends with suffix.
func (f *fakeAPI) to(suffix string) []apiCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []apiCall
	for _, c := range f.calls {
		if strings.HasSuffix(c.Path, suffix) {
			out = append(out, c)
		}
	}
	return out
}

// order returns the sequence of "<jobID>:<op>" / "deregister" markers for the
// calls that matter to shutdown ordering.
func (f *fakeAPI) order() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for _, c := range f.calls {
		switch {
		case strings.HasSuffix(c.Path, "/complete"):
			out = append(out, "complete")
		case strings.HasSuffix(c.Path, "/fail"):
			out = append(out, "fail")
		case strings.HasSuffix(c.Path, "/deregister"):
			out = append(out, "deregister")
		}
	}
	return out
}

func (f *fakeAPI) newWorker(opts Options, retry httpx.RetryConfig) *Worker {
	f.t.Helper()
	transport := httpx.NewTransport(httpx.Config{BaseURL: f.server.URL, APIKey: "sp_test_key", Retry: retry})
	if opts.QueueName == "" {
		opts.QueueName = "q"
	}
	if opts.PollInterval == 0 {
		opts.PollInterval = 10 * time.Millisecond
	}
	if opts.LeaseDuration == 0 {
		opts.LeaseDuration = 30
	}
	return NewWorker(resources.NewJobsResource(transport), resources.NewWorkersResource(transport), opts)
}

func claimOne(id string, payload string, extra string) string {
	return `{"jobs":[{"id":"` + id + `","queue_name":"q","payload":` + payload + `,"retry_count":0,"max_retries":3,` +
		`"timeout_seconds":0,"lease_id":"lease-` + id + `"` + extra + `}]}`
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func errorOf(c apiCall) string {
	s, _ := c.Body["error"].(string)
	return s
}

// --- SDKGP-02: panics --------------------------------------------------------

func TestWorker_RecoversHandlerPanicAndKeepsRunning(t *testing.T) {
	api := newFakeAPI(t, `{"jobs":[`+
		`{"id":"poison","queue_name":"q","payload":{"x":1},"retry_count":0,"max_retries":3,"timeout_seconds":0,"lease_id":"l1"},`+
		`{"id":"fine","queue_name":"q","payload":{"x":2},"retry_count":0,"max_retries":3,"timeout_seconds":0,"lease_id":"l2"}]}`)
	w := api.newWorker(Options{Concurrency: 2}, httpx.RetryConfig{})

	w.Process(func(ctx *JobContext) (map[string]any, error) {
		if ctx.JobID == "poison" {
			// README quick-start does exactly this type assertion on a payload
			// that has no such key.
			_ = ctx.Payload["missing"].(string)
		}
		return map[string]any{"ok": true}, nil
	})

	var failed []JobFailedData
	var mu sync.Mutex
	w.OnEvent(func(e Event) {
		if e.Type == EventJobFailed {
			mu.Lock()
			failed = append(failed, e.Data.(JobFailedData))
			mu.Unlock()
		}
	})

	if err := w.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = w.Stop() }()

	// The test process surviving to this line is already the main assertion: an
	// unrecovered panic on the job goroutine kills the whole test binary.
	// The job:failed event is emitted after the fail request returns, so wait
	// for it too instead of racing the worker goroutine.
	waitFor(t, "poison job failed and fine job completed", func() bool {
		mu.Lock()
		events := len(failed)
		mu.Unlock()
		return len(api.to("/jobs/poison/fail")) == 1 && len(api.to("/jobs/fine/complete")) == 1 && events >= 1
	})
	fail := api.to("/jobs/poison/fail")[0]
	if msg := errorOf(fail); !strings.HasPrefix(msg, "panic: ") || !strings.Contains(msg, "interface conversion") {
		t.Errorf("fail error = %q, want the panic text", msg)
	}
	if w.State() != StateRunning {
		t.Errorf("worker state = %s after a handler panic, want running", w.State())
	}
	mu.Lock()
	defer mu.Unlock()
	if len(failed) != 1 || failed[0].JobID != "poison" {
		t.Errorf("failed events = %+v, want one for poison", failed)
	}
}

func TestWorker_RecoversRuntimeGoexit(t *testing.T) {
	api := newFakeAPI(t, claimOne("exit", `{}`, ""))
	w := api.newWorker(Options{}, httpx.RetryConfig{})
	w.Process(func(*JobContext) (map[string]any, error) {
		// runtime.Goexit unwinds the goroutine with neither a return nor a panic
		// (t.FailNow does this); the job must still be settled.
		runtime.Goexit()
		return nil, nil
	})
	if err := w.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = w.Stop() }()

	waitFor(t, "job failed", func() bool { return len(api.to("/jobs/exit/fail")) == 1 })
}

// --- SDKGP-03 + brief item 1: graceful drain ---------------------------------

func TestWorker_StopDrainsRunningHandlerAndCompletesIt(t *testing.T) {
	api := newFakeAPI(t, claimOne("slow", `{}`, ""))
	w := api.newWorker(Options{ShutdownTimeout: 5 * time.Second}, httpx.RetryConfig{})

	started := make(chan struct{})
	release := make(chan struct{})
	var ctxCanceled atomic.Bool
	w.Process(func(ctx *JobContext) (map[string]any, error) {
		close(started)
		select {
		case <-release:
			return map[string]any{"done": true}, nil
		case <-ctx.Context.Done():
			ctxCanceled.Store(true)
			return nil, ctx.Context.Err()
		}
	})
	if err := w.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	<-started

	stopped := make(chan error, 1)
	go func() { stopped <- w.Stop() }()

	// While draining: not returned, handler context live, nothing settled yet.
	select {
	case <-stopped:
		t.Fatal("Stop returned while a handler was still running")
	case <-time.After(300 * time.Millisecond):
	}
	if ctxCanceled.Load() {
		t.Fatal("Stop canceled the in-flight job's context before the shutdown timeout")
	}
	if got := api.order(); len(got) != 0 {
		t.Fatalf("settled/deregistered during the drain: %v", got)
	}
	if st := w.State(); st != StateStopping {
		t.Errorf("state = %s while draining, want stopping", st)
	}

	close(release)
	select {
	case err := <-stopped:
		if err != nil {
			t.Fatalf("Stop: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Stop did not return after the handler finished")
	}

	// The handler's result is acknowledged, and only then does the worker leave.
	if got := strings.Join(api.order(), ","); got != "complete,deregister" {
		t.Errorf("call order = %s, want complete,deregister (and no fail)", got)
	}
	complete := api.to("/jobs/slow/complete")[0]
	if complete.Body["lease_id"] != "lease-slow" {
		t.Errorf("complete body = %v", complete.Body)
	}
	if w.State() != StateStopped {
		t.Errorf("state = %s, want stopped", w.State())
	}
}

func TestWorker_StopDoesNotClaimNewJobsWhileDraining(t *testing.T) {
	api := newFakeAPI(t, claimOne("slow", `{}`, ""))
	w := api.newWorker(Options{ShutdownTimeout: 5 * time.Second}, httpx.RetryConfig{})

	started := make(chan struct{})
	release := make(chan struct{})
	w.Process(func(*JobContext) (map[string]any, error) {
		close(started)
		<-release
		return nil, nil
	})
	if err := w.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	<-started

	go func() { _ = w.Stop() }()
	waitFor(t, "stopping state", func() bool { return w.State() == StateStopping })
	before := len(api.to("/jobs/claim"))
	time.Sleep(150 * time.Millisecond) // many poll intervals
	if after := len(api.to("/jobs/claim")); after != before {
		t.Errorf("claim polls went from %d to %d while draining; claiming must stop", before, after)
	}
	close(release)
	waitFor(t, "stopped", func() bool { return w.State() == StateStopped })
}

func TestWorker_CancellingStartContextDrainsLikeStop(t *testing.T) {
	api := newFakeAPI(t, claimOne("slow", `{}`, ""))
	w := api.newWorker(Options{ShutdownTimeout: 5 * time.Second}, httpx.RetryConfig{})

	started := make(chan struct{})
	release := make(chan struct{})
	var ctxCanceled atomic.Bool
	w.Process(func(ctx *JobContext) (map[string]any, error) {
		close(started)
		select {
		case <-release:
			return map[string]any{"done": true}, nil
		case <-ctx.Context.Done():
			ctxCanceled.Store(true)
			return nil, ctx.Context.Err()
		}
	})
	ctx, cancel := context.WithCancel(context.Background())
	if err := w.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	<-started

	// The README pattern: cancel the context, then Stop.
	cancel()
	waitFor(t, "stopping state after cancel", func() bool { return w.State() != StateRunning })
	time.Sleep(200 * time.Millisecond)
	if ctxCanceled.Load() {
		t.Fatal("cancelling the Start context aborted the running handler")
	}

	stopped := make(chan struct{})
	go func() {
		_ = w.Stop() // joins the shutdown already in progress
		close(stopped)
	}()
	select {
	case <-stopped:
		t.Fatal("Stop returned before the drain finished")
	case <-time.After(150 * time.Millisecond):
	}
	close(release)
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("Stop did not return after the drain")
	}
	if got := strings.Join(api.order(), ","); got != "complete,deregister" {
		t.Errorf("call order = %s, want complete,deregister", got)
	}
}

func TestWorker_StopFailsStillRunningJobsAfterShutdownTimeout(t *testing.T) {
	for _, tc := range []struct {
		name        string
		honorsCtx   bool
		wantHandler string
	}{
		{"handler that honors its context", true, "context canceled"},
		{"handler that ignores its context", false, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := newFakeAPI(t, claimOne("stuck", `{}`, ""))
			w := api.newWorker(Options{ShutdownTimeout: 200 * time.Millisecond}, httpx.RetryConfig{})

			started := make(chan struct{})
			release := make(chan struct{})
			var ctxCanceledAt atomic.Int64
			w.Process(func(ctx *JobContext) (map[string]any, error) {
				close(started)
				if tc.honorsCtx {
					<-ctx.Context.Done()
					ctxCanceledAt.Store(time.Now().UnixNano())
					return nil, ctx.Context.Err()
				}
				<-release
				return map[string]any{"late": true}, nil
			})
			if err := w.Start(context.Background()); err != nil {
				t.Fatalf("Start: %v", err)
			}
			<-started

			begin := time.Now()
			if err := w.Stop(); err != nil {
				t.Fatalf("Stop: %v", err)
			}
			if elapsed := time.Since(begin); elapsed < 180*time.Millisecond || elapsed > 3*time.Second {
				t.Errorf("Stop took %v, want about the 200ms shutdown timeout", elapsed)
			}
			if tc.honorsCtx && ctxCanceledAt.Load() == 0 {
				t.Error("the handler's context was never canceled after the timeout")
			}

			fails := api.to("/jobs/stuck/fail")
			if len(fails) != 1 || errorOf(fails[0]) != "Worker shutdown timeout" {
				t.Fatalf("fail calls = %+v, want exactly one with \"Worker shutdown timeout\"", fails)
			}
			if got := strings.Join(api.order(), ","); got != "fail,deregister" {
				t.Errorf("call order = %s, want fail,deregister (deregister last)", got)
			}

			// A handler that eventually returns must not settle the job a second time.
			close(release)
			time.Sleep(100 * time.Millisecond)
			if got := strings.Join(api.order(), ","); got != "fail,deregister" {
				t.Errorf("late handler return changed the calls: %s", got)
			}
		})
	}
}

// --- Item 5: per-job timeout -------------------------------------------------

func TestWorker_JobTimeoutCancelsHandlerAndFailsJob(t *testing.T) {
	for _, honorsCtx := range []bool{true, false} {
		name := "handler ignores ctx"
		if honorsCtx {
			name = "handler honors ctx"
		}
		t.Run(name, func(t *testing.T) {
			api := newFakeAPI(t, `{"jobs":[{"id":"hung","queue_name":"q","payload":{},"retry_count":0,"max_retries":3,`+
				`"timeout_seconds":1,"lease_id":"l-hung"}]}`)
			w := api.newWorker(Options{Concurrency: 1, LeaseDuration: 2, HeartbeatFraction: 0.5}, httpx.RetryConfig{})

			release := make(chan struct{})
			defer close(release)
			var sawCancel atomic.Bool
			w.Process(func(ctx *JobContext) (map[string]any, error) {
				if honorsCtx {
					<-ctx.Context.Done()
					sawCancel.Store(true)
					return nil, ctx.Context.Err()
				}
				<-release
				return nil, nil
			})

			var timeouts []JobTimeoutData
			var mu sync.Mutex
			w.OnEvent(func(e Event) {
				if e.Type == EventJobTimeout {
					mu.Lock()
					timeouts = append(timeouts, e.Data.(JobTimeoutData))
					mu.Unlock()
				}
			})

			start := time.Now()
			if err := w.Start(context.Background()); err != nil {
				t.Fatalf("Start: %v", err)
			}
			defer func() { _ = w.Stop() }()

			waitFor(t, "timeout failure", func() bool { return len(api.to("/jobs/hung/fail")) >= 1 })
			if elapsed := time.Since(start); elapsed < 900*time.Millisecond {
				t.Errorf("job failed after %v, before its 1s timeout", elapsed)
			}
			fails := api.to("/jobs/hung/fail")
			if len(fails) != 1 || errorOf(fails[0]) != "Job timeout exceeded" {
				t.Fatalf("fail calls = %+v, want exactly one \"Job timeout exceeded\"", fails)
			}
			if honorsCtx {
				waitFor(t, "handler context canceled", sawCancel.Load)
			}
			mu.Lock()
			if len(timeouts) != 1 || timeouts[0].JobID != "hung" || timeouts[0].TimeoutSeconds != 1 {
				t.Errorf("job:timeout events = %+v", timeouts)
			}
			mu.Unlock()

			// The slot is released even though the handler is still stuck, and the
			// lease is no longer extended for it.
			waitFor(t, "slot released", func() bool { return w.ActiveJobCount() == 0 })
			beats := len(api.to("/jobs/hung/heartbeat"))
			time.Sleep(1300 * time.Millisecond)
			if after := len(api.to("/jobs/hung/heartbeat")); after != beats {
				t.Errorf("lease renewals continued after the timeout: %d -> %d", beats, after)
			}
		})
	}
}

// --- SDKGP-12: failure text --------------------------------------------------

func TestWorker_FailureTextIsAlwaysAcceptable(t *testing.T) {
	api := newFakeAPI(t, `{"jobs":[`+
		`{"id":"empty","queue_name":"q","payload":{},"retry_count":0,"max_retries":3,"timeout_seconds":0,"lease_id":"l1"},`+
		`{"id":"long","queue_name":"q","payload":{},"retry_count":0,"max_retries":3,"timeout_seconds":0,"lease_id":"l2"}]}`)
	w := api.newWorker(Options{Concurrency: 2}, httpx.RetryConfig{})
	w.Process(func(ctx *JobContext) (map[string]any, error) {
		if ctx.JobID == "empty" {
			return nil, errors.New("")
		}
		return nil, errors.New(strings.Repeat("upstream said: 日本語 ", 400))
	})
	if err := w.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = w.Stop() }()

	waitFor(t, "both failures", func() bool {
		return len(api.to("/jobs/empty/fail")) == 1 && len(api.to("/jobs/long/fail")) == 1
	})
	if got := errorOf(api.to("/jobs/empty/fail")[0]); got != "Job failed" {
		t.Errorf("empty error sent as %q, want %q", got, "Job failed")
	}
	long := errorOf(api.to("/jobs/long/fail")[0])
	if n := len([]rune(long)); n != 2048 {
		t.Errorf("long error sent with %d characters, want 2048", n)
	}
	if !strings.HasPrefix(long, "upstream said:") {
		t.Errorf("long error was not truncated from the end: %.30q", long)
	}
}

// --- SDKGP-11: non-object payloads -------------------------------------------

func TestWorker_NonObjectPayloadsReachHandlers(t *testing.T) {
	for name, tc := range map[string]struct {
		payload string
		check   func(t *testing.T, raw any, obj map[string]any)
	}{
		"array": {`[1,2,3]`, func(t *testing.T, raw any, obj map[string]any) {
			if arr, ok := raw.([]any); !ok || len(arr) != 3 {
				t.Errorf("RawPayload = %#v, want a 3-element array", raw)
			}
			if obj != nil {
				t.Errorf("Payload = %v, want nil for a non-object", obj)
			}
		}},
		"string": {`"hello"`, func(t *testing.T, raw any, _ map[string]any) {
			if raw != "hello" {
				t.Errorf("RawPayload = %#v", raw)
			}
		}},
		"number": {`7`, func(t *testing.T, raw any, _ map[string]any) {
			if raw != float64(7) {
				t.Errorf("RawPayload = %#v", raw)
			}
		}},
		"object": {`{"a":1,"id":1234567890123456789}`, func(t *testing.T, raw any, obj map[string]any) {
			if obj["a"] != float64(1) {
				t.Errorf("Payload[a] = %#v, want float64(1)", obj["a"])
			}
			if n, ok := obj["id"].(json.Number); !ok || n.String() != "1234567890123456789" {
				t.Errorf("Payload[id] = %#v, want an exact json.Number", obj["id"])
			}
			if m, ok := raw.(map[string]any); !ok || len(m) != 2 {
				t.Errorf("RawPayload = %#v", raw)
			}
		}},
	} {
		t.Run(name, func(t *testing.T) {
			api := newFakeAPI(t, claimOne("p", tc.payload, ""))
			w := api.newWorker(Options{}, httpx.RetryConfig{})
			type seen struct {
				raw any
				obj map[string]any
			}
			got := make(chan seen, 1)
			w.Process(func(ctx *JobContext) (map[string]any, error) {
				got <- seen{ctx.RawPayload, ctx.Payload}
				return nil, nil
			})
			if err := w.Start(context.Background()); err != nil {
				t.Fatalf("Start: %v", err)
			}
			defer func() { _ = w.Stop() }()
			select {
			case s := <-got:
				tc.check(t, s.raw, s.obj)
			case <-time.After(5 * time.Second):
				t.Fatal("handler never ran")
			}
		})
	}
}

// --- Item 2: acks are retried ------------------------------------------------

func TestWorker_CompleteIsRetriedOnTransientFailure(t *testing.T) {
	api := newFakeAPI(t, claimOne("j", `{}`, ""))
	api.respond = func(path string, nth int, w http.ResponseWriter) bool {
		if strings.HasSuffix(path, "/complete") && nth <= 2 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return true
		}
		return false
	}
	retry := httpx.RetryConfig{MaxRetries: 3, BaseDelay: time.Millisecond, MaxDelay: 5 * time.Millisecond, Factor: 2}
	w := api.newWorker(Options{}, retry)
	w.Process(func(*JobContext) (map[string]any, error) { return map[string]any{"ok": true}, nil })

	var completed, errs atomic.Int32
	w.OnEvent(func(e Event) {
		switch e.Type {
		case EventJobCompleted:
			completed.Add(1)
		case EventWorkerError:
			errs.Add(1)
		}
	})
	if err := w.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = w.Stop() }()

	waitFor(t, "completed event", func() bool { return completed.Load() == 1 })
	if got := len(api.to("/jobs/j/complete")); got != 3 {
		t.Errorf("complete attempts = %d, want 3 (two 503s then success)", got)
	}
	if errs.Load() != 0 {
		t.Errorf("worker errors emitted for a recovered ack: %d", errs.Load())
	}
}

// A retried ack whose first attempt actually landed meets "Job is already
// completed"; that is success, not a worker error.
func TestWorker_AlreadySettledAckCountsAsSuccess(t *testing.T) {
	api := newFakeAPI(t, claimOne("j", `{}`, ""))
	api.respond = func(path string, nth int, w http.ResponseWriter) bool {
		if strings.HasSuffix(path, "/complete") {
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`{"code":"CONFLICT","message":"Job is already completed"}`))
			return true
		}
		return false
	}
	w := api.newWorker(Options{}, httpx.RetryConfig{})
	w.Process(func(*JobContext) (map[string]any, error) { return nil, nil })

	var completed, errs, leaseLost atomic.Int32
	w.OnEvent(func(e Event) {
		switch e.Type {
		case EventJobCompleted:
			completed.Add(1)
		case EventWorkerError:
			errs.Add(1)
		case EventJobLeaseLost:
			leaseLost.Add(1)
		}
	})
	if err := w.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = w.Stop() }()

	waitFor(t, "completed event", func() bool { return completed.Load() == 1 })
	if errs.Load() != 0 || leaseLost.Load() != 0 {
		t.Errorf("errors=%d leaseLost=%d, want 0/0", errs.Load(), leaseLost.Load())
	}
}

func TestIsAlreadySettled(t *testing.T) {
	parse := func(status int, body string) error {
		return httpx.ParseErrorFromResponse(status, []byte(body), http.Header{})
	}
	if !isAlreadySettled(parse(409, `{"code":"CONFLICT","message":"Job is already completed"}`)) {
		t.Error("409 'Job is already completed' should be treated as settled")
	}
	if isAlreadySettled(parse(409, `{"code":"LEASE_EXPIRED","message":"Job is already gone"}`)) {
		t.Error("a LEASE_EXPIRED conflict is a lost lease, not a settled job")
	}
	if isAlreadySettled(parse(409, `{"code":"CONFLICT","message":"something else"}`)) {
		t.Error("an unrelated conflict must not be swallowed")
	}
	if isAlreadySettled(parse(500, `{"message":"Job is already completed"}`)) {
		t.Error("only 409s qualify")
	}
}

// --- SDKGP-10: Progress ------------------------------------------------------

func TestWorker_ProgressIsUnsupportedAndMakesNoRequest(t *testing.T) {
	api := newFakeAPI(t, claimOne("j", `{}`, ""))
	w := api.newWorker(Options{}, httpx.RetryConfig{})
	got := make(chan error, 1)
	w.Process(func(ctx *JobContext) (map[string]any, error) {
		got <- ctx.Progress(50, "half way")
		return nil, nil
	})
	if err := w.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = w.Stop() }()

	select {
	case err := <-got:
		var unsupported *resources.UnsupportedError
		if !errors.Is(err, resources.ErrUnsupported) || !errors.As(err, &unsupported) || unsupported.Operation != "JobContext.Progress" {
			t.Fatalf("Progress err = %v, want a typed unsupported error", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("handler never ran")
	}
	waitFor(t, "complete", func() bool { return len(api.to("/jobs/j/complete")) == 1 })
	if n := len(api.to("/progress")); n != 0 {
		t.Errorf("Progress made %d progress requests", n)
	}
}

// --- Start / config robustness ----------------------------------------------

func TestWorker_StartCanBeRetriedAfterFailedRegistration(t *testing.T) {
	api := newFakeAPI(t, `{"jobs":[]}`)
	api.respond = func(path string, nth int, w http.ResponseWriter) bool {
		if path == "/api/v1/workers/register" && nth == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"code":"SERVICE_UNAVAILABLE","message":"try later"}`))
			return true
		}
		return false
	}
	w := api.newWorker(Options{}, httpx.RetryConfig{})
	w.Process(func(*JobContext) (map[string]any, error) { return nil, nil })

	if err := w.Start(context.Background()); err == nil {
		t.Fatal("first Start should report the registration failure")
	}
	if w.State() != StateError {
		t.Errorf("state after failed Start = %s, want error", w.State())
	}
	if err := w.Start(context.Background()); err != nil {
		t.Fatalf("second Start should retry registration and succeed: %v", err)
	}
	defer func() { _ = w.Stop() }()
	waitFor(t, "claim polls", func() bool { return len(api.to("/jobs/claim")) > 0 })
}

func TestWorker_ShortLeaseDoesNotPanicTickers(t *testing.T) {
	api := newFakeAPI(t, claimOne("j", `{}`, ""))
	// 5s * 0.1 = 0.5s used to truncate to a zero-length ticker, which panics.
	w := api.newWorker(Options{LeaseDuration: 5, HeartbeatFraction: 0.1}, httpx.RetryConfig{})
	if d := w.heartbeatInterval(); d < time.Second {
		t.Errorf("heartbeatInterval = %v, want at least 1s", d)
	}
	w.Process(func(*JobContext) (map[string]any, error) { return nil, nil })
	if err := w.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitFor(t, "complete", func() bool { return len(api.to("/jobs/j/complete")) == 1 })
	_ = w.Stop()
}

func TestWorker_ClaimLimitIsCappedAtTheAPIMaximum(t *testing.T) {
	api := newFakeAPI(t, `{"jobs":[]}`)
	w := api.newWorker(Options{Concurrency: 250}, httpx.RetryConfig{})
	w.Process(func(*JobContext) (map[string]any, error) { return nil, nil })
	if err := w.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = w.Stop() }()

	waitFor(t, "claim", func() bool { return len(api.to("/jobs/claim")) > 0 })
	if limit := api.to("/jobs/claim")[0].Body["limit"]; limit != float64(100) {
		t.Errorf("claim limit = %v, want 100 (the API rejects more with 422)", limit)
	}
}

func TestWorker_StopIsIdempotentAndSafeBeforeStart(t *testing.T) {
	api := newFakeAPI(t, `{"jobs":[]}`)
	w := api.newWorker(Options{}, httpx.RetryConfig{})
	w.Process(func(*JobContext) (map[string]any, error) { return nil, nil })

	if err := w.Stop(); err != nil { // before Start: a no-op that must not poison a later Start
		t.Fatalf("Stop before Start: %v", err)
	}
	if err := w.Start(context.Background()); err != nil {
		t.Fatalf("Start after an early Stop: %v", err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = w.Stop()
		}()
	}
	wg.Wait()
	if len(api.to("/deregister")) != 1 {
		t.Errorf("deregister calls = %d, want exactly 1", len(api.to("/deregister")))
	}
}

// TestWorker_StopDoesNotLogCanceledHeartbeat: a worker heartbeat that is in
// flight when Stop ends the heartbeat loop is aborted on purpose. It used to be
// logged as "Failed to send worker heartbeat: context canceled" on every
// graceful shutdown (examples/worker showed it).
func TestWorker_StopDoesNotLogCanceledHeartbeat(t *testing.T) {
	api := newFakeAPI(t, `{"jobs":[]}`)
	release := make(chan struct{})
	t.Cleanup(func() { close(release) }) // runs before the server closes
	api.respond = func(path string, _ int, _ http.ResponseWriter) bool {
		if strings.HasSuffix(path, "/workers/w-test/heartbeat") {
			<-release // hold the heartbeat until the worker gives up on it
			return true
		}
		return false
	}

	var mu sync.Mutex
	var logs []string
	w := api.newWorker(Options{
		LeaseDuration:     1, // heartbeat every second (the minimum)
		HeartbeatFraction: 0.5,
		Logger: func(msg string, args ...any) {
			mu.Lock()
			defer mu.Unlock()
			logs = append(logs, fmt.Sprintf(msg, args...))
		},
	}, httpx.RetryConfig{MaxRetries: 2, BaseDelay: 10 * time.Millisecond, MaxDelay: 20 * time.Millisecond})
	w.Process(func(*JobContext) (map[string]any, error) { return nil, nil })
	if err := w.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitFor(t, "a worker heartbeat in flight", func() bool { return len(api.to("/workers/w-test/heartbeat")) > 0 })

	if err := w.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	for _, line := range logs {
		if strings.Contains(line, "Failed to send worker heartbeat") {
			t.Errorf("graceful shutdown logged %q", line)
		}
	}
	if len(api.to("/deregister")) != 1 {
		t.Errorf("deregister calls = %d, want 1", len(api.to("/deregister")))
	}
}
