package worker

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/spooled-cloud/spooled-sdk-go/internal/httpx"
	"github.com/spooled-cloud/spooled-sdk-go/spooled/resources"
)

// leaseServer is a fake Spooled REST backend that hands out one batch of
// claimed jobs and records the bodies of every job complete/fail/heartbeat
// call so tests can assert the lease_id fencing token round-trip.
type leaseServer struct {
	mu     sync.Mutex
	jobs   string // JSON claim response body, served once
	served bool
	bodies map[string][]map[string]any // "<jobID>:<op>" -> request bodies
}

func (s *leaseServer) record(key string, body []byte) {
	var decoded map[string]any
	_ = json.Unmarshal(body, &decoded)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.bodies[key] = append(s.bodies[key], decoded)
}

func (s *leaseServer) recorded(key string) []map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]map[string]any(nil), s.bodies[key]...)
}

func (s *leaseServer) handler(t *testing.T) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")

		path := r.URL.Path
		switch {
		case path == "/api/v1/workers/register":
			_, _ = w.Write([]byte(`{"id":"w-test","queue_name":"q","lease_duration_secs":2,"heartbeat_interval_secs":1}`))
		case path == "/api/v1/jobs/claim":
			s.mu.Lock()
			jobs := s.jobs
			if s.served {
				jobs = `{"jobs":[]}`
			}
			s.served = true
			s.mu.Unlock()
			_, _ = w.Write([]byte(jobs))
		case strings.HasPrefix(path, "/api/v1/jobs/"):
			parts := strings.Split(strings.TrimPrefix(path, "/api/v1/jobs/"), "/")
			if len(parts) == 2 {
				s.record(parts[0]+":"+parts[1], body)
			}
			_, _ = w.Write([]byte(`{"success":true}`))
		case strings.HasPrefix(path, "/api/v1/workers/"):
			// Worker heartbeat / deregister.
			_, _ = w.Write([]byte(`{}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, path)
			w.WriteHeader(http.StatusNotFound)
		}
	})
}

func newTestWorker(t *testing.T, handler http.Handler) (*Worker, func()) {
	t.Helper()
	server := httptest.NewServer(handler)
	transport := httpx.NewTransport(httpx.Config{
		BaseURL: server.URL,
		APIKey:  "sp_test_key",
	})
	w := NewWorker(
		resources.NewJobsResource(transport),
		resources.NewWorkersResource(transport),
		Options{QueueName: "q", LeaseDuration: 30},
	)
	w.workerID = "w-test"
	w.ctx = context.Background()
	return w, server.Close
}

// TestWorker_EchoesLeaseID verifies that the fencing token returned by claim
// is echoed on complete, fail, and heartbeat (lease renewal), and that jobs
// claimed without a token (legacy backend) omit lease_id entirely.
func TestWorker_EchoesLeaseID(t *testing.T) {
	srv := &leaseServer{
		bodies: make(map[string][]map[string]any),
		jobs: `{"jobs":[
			{"id":"job-ok","queue_name":"q","payload":{"outcome":"ok"},"retry_count":0,"max_retries":3,"timeout_seconds":30,"lease_id":"lease-ok"},
			{"id":"job-bad","queue_name":"q","payload":{"outcome":"fail"},"retry_count":0,"max_retries":3,"timeout_seconds":30,"lease_id":"lease-bad"},
			{"id":"job-legacy","queue_name":"q","payload":{"outcome":"ok"},"retry_count":0,"max_retries":3,"timeout_seconds":30}
		]}`,
	}
	server := httptest.NewServer(srv.handler(t))
	defer server.Close()

	transport := httpx.NewTransport(httpx.Config{
		BaseURL: server.URL,
		APIKey:  "sp_test_key",
	})

	w := NewWorker(
		resources.NewJobsResource(transport),
		resources.NewWorkersResource(transport),
		Options{
			QueueName:    "q",
			Concurrency:  3,
			PollInterval: 25 * time.Millisecond,
			// LeaseDuration 2s at fraction 0.5 gives a 1s job-heartbeat
			// ticker; the handler runs longer so each job renews once.
			LeaseDuration:     2,
			HeartbeatFraction: 0.5,
		},
	)

	done := make(chan string, 3)
	w.Process(func(ctx *JobContext) (map[string]any, error) {
		defer func() { done <- ctx.JobID }()
		time.Sleep(1300 * time.Millisecond) // allow one lease renewal tick
		if ctx.Payload["outcome"] == "fail" {
			return nil, errors.New("boom")
		}
		return map[string]any{"ok": true}, nil
	})

	if err := w.Start(context.Background()); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	for i := 0; i < 3; i++ {
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Fatal("timed out waiting for jobs to finish")
		}
	}
	if err := w.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	assertLeaseID := func(op, jobID, want string) {
		t.Helper()
		bodies := srv.recorded(jobID + ":" + op)
		if len(bodies) == 0 {
			t.Errorf("no %s call recorded for %s", op, jobID)
			return
		}
		for _, body := range bodies {
			got, present := body["lease_id"]
			if want == "" {
				if present {
					t.Errorf("%s %s: lease_id should be omitted, got %v", op, jobID, got)
				}
				continue
			}
			if !present {
				t.Errorf("%s %s: lease_id missing, want %q", op, jobID, want)
				continue
			}
			if got != want {
				t.Errorf("%s %s: lease_id = %v, want %q", op, jobID, got, want)
			}
		}
	}

	assertLeaseID("complete", "job-ok", "lease-ok")
	assertLeaseID("fail", "job-bad", "lease-bad")
	assertLeaseID("heartbeat", "job-ok", "lease-ok")
	assertLeaseID("heartbeat", "job-bad", "lease-bad")
	// Legacy claim without a token must keep omitting lease_id.
	assertLeaseID("complete", "job-legacy", "")
	assertLeaseID("heartbeat", "job-legacy", "")
}

func TestWorker_ExecutionIdentitySurvivesReplacementLease(t *testing.T) {
	var gotLeaseID string
	w, closeServer := newTestWorker(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		gotLeaseID, _ = body["lease_id"].(string)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true}`))
	}))
	defer closeServer()

	oldLease, newLease := "lease-old", "lease-new"
	oldExecution := &activeJob{jobID: "same-job", queueName: "q", workerID: "w-test", leaseID: &oldLease}
	newExecution := &activeJob{jobID: "same-job", queueName: "q", workerID: "w-test", leaseID: &newLease}
	w.activeJobs.Store(oldExecution.jobID, oldExecution)
	w.activeJobs.Store(newExecution.jobID, newExecution)

	w.completeJob(oldExecution, map[string]any{"ok": true}, time.Second)
	w.activeJobs.CompareAndDelete(oldExecution.jobID, oldExecution)

	if gotLeaseID != oldLease {
		t.Fatalf("completion lease_id = %q, want immutable execution lease %q", gotLeaseID, oldLease)
	}
	got, ok := w.activeJobs.Load(newExecution.jobID)
	if !ok || got != newExecution {
		t.Fatal("old execution cleanup deleted the replacement lease")
	}
}

func TestWorker_LeaseExpiredHeartbeatCancelsExactExecution(t *testing.T) {
	w, closeServer := newTestWorker(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"code":"LEASE_EXPIRED","message":"stale lease"}`))
	}))
	defer closeServer()

	leaseID := "lease-stale"
	ctx, cancel := context.WithCancel(context.Background())
	aj := &activeJob{jobID: "job-1", queueName: "q", workerID: "w-test", leaseID: &leaseID, ctx: ctx, cancel: cancel}
	events := make(chan Event, 1)
	w.OnEvent(func(event Event) { events <- event })

	if w.renewJobLease(aj) {
		t.Fatal("lease-expired renewal should stop the renewal loop")
	}
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("lease-expired renewal did not cancel the handler context")
	}
	select {
	case event := <-events:
		if event.Type != EventJobLeaseLost {
			t.Fatalf("event type = %q, want %q", event.Type, EventJobLeaseLost)
		}
		data, ok := event.Data.(JobLeaseLostData)
		if !ok || data.LeaseID != leaseID || data.Operation != "renew lease" {
			t.Fatalf("unexpected lease-loss data: %#v", event.Data)
		}
	case <-time.After(time.Second):
		t.Fatal("missing lease-loss event")
	}
}

func TestWorker_LeaseLossAfterHandlerReturnSkipsSettlementAndDuplicateEvents(t *testing.T) {
	var mu sync.Mutex
	settlements := 0
	handlerStarted := make(chan struct{})
	handlerRelease := make(chan struct{})

	srv := &leaseServer{
		bodies: make(map[string][]map[string]any),
		jobs: `{"jobs":[{"id":"job-1","queue_name":"q","payload":{},` +
			`"retry_count":0,"max_retries":3,"timeout_seconds":30,"lease_id":"lease-stale"}]}`,
	}
	baseHandler := srv.handler(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/heartbeat") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`{"code":"LEASE_EXPIRED","message":"stale lease"}`))
			return
		}
		if strings.HasSuffix(r.URL.Path, "/complete") || strings.HasSuffix(r.URL.Path, "/fail") {
			mu.Lock()
			settlements++
			mu.Unlock()
		}
		baseHandler.ServeHTTP(w, r)
	}))
	defer server.Close()

	transport := httpx.NewTransport(httpx.Config{BaseURL: server.URL, APIKey: "sp_test_key"})
	w := NewWorker(
		resources.NewJobsResource(transport),
		resources.NewWorkersResource(transport),
		Options{QueueName: "q", Concurrency: 1, PollInterval: 20 * time.Millisecond, LeaseDuration: 2, HeartbeatFraction: 0.5},
	)
	var eventMu sync.Mutex
	var events []Event
	w.OnEvent(func(event Event) {
		eventMu.Lock()
		events = append(events, event)
		eventMu.Unlock()
	})
	w.Process(func(ctx *JobContext) (map[string]any, error) {
		close(handlerStarted)
		<-ctx.Context.Done()
		<-handlerRelease
		return map[string]any{"stale": true}, nil
	})

	if err := w.Start(context.Background()); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	select {
	case <-handlerStarted:
	case <-time.After(time.Second):
		t.Fatal("handler did not start")
	}

	deadline := time.After(3 * time.Second)
	for {
		eventMu.Lock()
		leaseLost := 0
		for _, event := range events {
			if event.Type == EventJobLeaseLost {
				leaseLost++
			}
		}
		eventMu.Unlock()
		if leaseLost == 1 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for lease-loss event")
		case <-time.After(10 * time.Millisecond):
		}
	}

	close(handlerRelease)
	deadline = time.After(3 * time.Second)
	for w.ActiveJobCount() != 0 {
		select {
		case <-deadline:
			t.Fatal("handler did not finish")
		case <-time.After(10 * time.Millisecond):
		}
	}
	if err := w.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	mu.Lock()
	gotSettlements := settlements
	mu.Unlock()
	if gotSettlements != 0 {
		t.Fatalf("sent %d settlement requests after lease loss, want 0", gotSettlements)
	}
	eventMu.Lock()
	defer eventMu.Unlock()
	leaseLost, completed, failed := 0, 0, 0
	for _, event := range events {
		switch event.Type {
		case EventJobLeaseLost:
			leaseLost++
		case EventJobCompleted:
			completed++
		case EventJobFailed:
			failed++
		}
	}
	if leaseLost != 1 || completed != 0 || failed != 0 {
		t.Fatalf("leaseLost=%d completed=%d failed=%d, want 1/0/0", leaseLost, completed, failed)
	}
}

func TestWorker_FailedEventWillRetry(t *testing.T) {
	tests := []struct {
		name          string
		response      string
		fallback      bool
		wantWillRetry bool
	}{
		{name: "backend true overrides fallback", response: `{"success":true,"will_retry":true}`, fallback: false, wantWillRetry: true},
		{name: "backend false overrides fallback", response: `{"success":true,"will_retry":false}`, fallback: true, wantWillRetry: false},
		{name: "claim retry state fallback", response: `{"success":true}`, fallback: true, wantWillRetry: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w, closeServer := newTestWorker(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tt.response))
			}))
			defer closeServer()

			leaseID := "lease-1"
			maxRetries := 0
			if tt.fallback {
				maxRetries = 1
			}
			aj := &activeJob{
				jobID: "job-1", queueName: "q", workerID: "w-test", leaseID: &leaseID,
				retryCount: 0, maxRetries: maxRetries,
			}
			var got JobFailedData
			w.OnEvent(func(event Event) {
				if event.Type == EventJobFailed {
					got = event.Data.(JobFailedData)
				}
			})

			w.failJob(aj, errors.New("handler failed"), time.Second)
			if got.WillRetry != tt.wantWillRetry {
				t.Fatalf("WillRetry = %v, want %v", got.WillRetry, tt.wantWillRetry)
			}
		})
	}
}

func TestWorker_SettlementEventsRequireBackendConfirmation(t *testing.T) {
	tests := []struct {
		name       string
		operation  string
		wantAbsent EventType
		settle     func(*Worker, *activeJob)
	}{
		{
			name:       "complete rejected",
			operation:  "complete",
			wantAbsent: EventJobCompleted,
			settle: func(w *Worker, aj *activeJob) {
				w.completeJob(aj, map[string]any{"ok": true}, time.Second)
			},
		},
		{
			name:       "fail rejected",
			operation:  "fail",
			wantAbsent: EventJobFailed,
			settle: func(w *Worker, aj *activeJob) {
				w.failJob(aj, errors.New("handler failed"), time.Second)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w, closeServer := newTestWorker(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"code":"SETTLEMENT_FAILED","message":"try later"}`))
			}))
			defer closeServer()

			leaseID := "lease-1"
			aj := &activeJob{jobID: "job-1", queueName: "q", workerID: "w-test", leaseID: &leaseID}
			var events []Event
			w.OnEvent(func(event Event) { events = append(events, event) })

			tt.settle(w, aj)

			if len(events) != 1 || events[0].Type != EventWorkerError {
				t.Fatalf("events = %#v, want one worker:error for rejected %s", events, tt.operation)
			}
			for _, event := range events {
				if event.Type == tt.wantAbsent {
					t.Fatalf("emitted %q before backend confirmed settlement", tt.wantAbsent)
				}
			}
		})
	}
}
