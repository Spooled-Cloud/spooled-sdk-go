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
