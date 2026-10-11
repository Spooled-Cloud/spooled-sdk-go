package spooled

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	googlegrpc "google.golang.org/grpc"

	spooledgrpc "github.com/spooled-cloud/spooled-sdk-go/spooled/grpc"
	"github.com/spooled-cloud/spooled-sdk-go/spooled/grpc/pb"
	"github.com/spooled-cloud/spooled-sdk-go/spooled/resources"
)

const testAPIKey = "sp_test_123456789012345678901234567890"

// --- SDKGP-09: Start after a failed registration ----------------------------

func TestSpooledWorker_StartRetriesAfterFailedRegistration(t *testing.T) {
	var registers, claims atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/workers/register":
			if registers.Add(1) == 1 {
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = w.Write([]byte(`{"code":"SERVICE_UNAVAILABLE","message":"try later"}`))
				return
			}
			_, _ = w.Write([]byte(`{"id":"w1","queue_name":"q","lease_duration_secs":30,"heartbeat_interval_secs":15}`))
		case "/api/v1/jobs/claim":
			claims.Add(1)
			_, _ = w.Write([]byte(`{"jobs":[]}`))
		default:
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	defer server.Close()

	client, err := NewClient(WithAPIKey(testAPIKey), WithBaseURL(server.URL), WithRetry(RetryConfig{MaxRetries: 0}))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	w := NewSpooledWorker(client, SpooledWorkerOptions{QueueName: "q", PollInterval: 10 * time.Millisecond})
	w.Process(func(context.Context, *resources.Job) (any, error) { return nil, nil })

	if err := w.Start(); err == nil {
		t.Fatal("first Start should return the registration error")
	}
	// The retry loop a caller writes: the second Start used to return nil
	// ("already started") while nothing was polling.
	if err := w.Start(); err != nil {
		t.Fatalf("second Start: %v", err)
	}
	defer func() { _ = w.Stop() }()

	deadline := time.Now().Add(3 * time.Second)
	for claims.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if claims.Load() == 0 {
		t.Fatal("after the retried Start nothing polls for jobs")
	}
	if registers.Load() != 2 {
		t.Errorf("registration attempts = %d, want 2", registers.Load())
	}
	// A running worker's Start stays a no-op.
	if err := w.Start(); err != nil {
		t.Errorf("Start on a running worker = %v, want nil", err)
	}
}

// --- SDKGP-11: SpooledWorker sees non-object payloads ------------------------

func TestSpooledWorker_HandlerSeesNonObjectPayload(t *testing.T) {
	var served atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/workers/register":
			_, _ = w.Write([]byte(`{"id":"w1","queue_name":"q","lease_duration_secs":30,"heartbeat_interval_secs":15}`))
		case "/api/v1/jobs/claim":
			if served.Swap(true) {
				_, _ = w.Write([]byte(`{"jobs":[]}`))
				return
			}
			_, _ = w.Write([]byte(`{"jobs":[{"id":"j1","queue_name":"q","payload":[1,2,3],"retry_count":0,"max_retries":3,"timeout_seconds":0}]}`))
		default:
			_, _ = w.Write([]byte(`{"success":true}`))
		}
	}))
	defer server.Close()

	client, err := NewClient(WithAPIKey(testAPIKey), WithBaseURL(server.URL))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	w := NewSpooledWorker(client, SpooledWorkerOptions{QueueName: "q", PollInterval: 10 * time.Millisecond, ShutdownTimeout: time.Second})
	got := make(chan any, 1)
	w.Process(func(_ context.Context, job *resources.Job) (any, error) {
		got <- job.Payload
		return nil, nil
	})
	if err := w.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = w.Stop() }()

	select {
	case payload := <-got:
		if arr, ok := payload.([]any); !ok || len(arr) != 3 {
			t.Errorf("handler payload = %#v, want [1 2 3] (it used to be a nil map)", payload)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("handler never ran")
	}
}

// --- SDKGP-05: Client.GRPC() TLS options, and Close closes the connection ----

type enqueueOnlyServer struct {
	pb.UnimplementedQueueServiceServer
	mu       sync.Mutex
	payloads []map[string]any
}

func (s *enqueueOnlyServer) Enqueue(_ context.Context, req *pb.EnqueueRequest) (*pb.EnqueueResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.payloads = append(s.payloads, req.GetPayload().AsMap())
	return &pb.EnqueueResponse{JobId: "j1", Created: true}, nil
}

func startLocalGRPC(t *testing.T) (string, *enqueueOnlyServer) {
	t.Helper()
	listener, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := googlegrpc.NewServer()
	impl := &enqueueOnlyServer{}
	pb.RegisterQueueServiceServer(srv, impl)
	go func() { _ = srv.Serve(listener) }()
	t.Cleanup(srv.Stop)
	_, port, _ := net.SplitHostPort(listener.Addr().String())
	return "localhost:" + port, impl
}

func TestClientGRPC_PlaintextLocalhostWorksAndTLSCanBeForced(t *testing.T) {
	addr, impl := startLocalGRPC(t)

	client, err := NewClient(WithAPIKey(testAPIKey), WithGRPCAddress(addr))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	g, err := client.GRPC()
	if err != nil {
		t.Fatalf("GRPC(): %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := g.Enqueue(ctx, &spooledgrpc.EnqueueRequest{QueueName: "q", Payload: map[string]any{"tags": []string{"a"}}}); err != nil {
		t.Fatalf("Enqueue against localhost: %v", err)
	}
	if len(impl.payloads) != 1 || impl.payloads[0]["tags"] == nil {
		t.Errorf("server payloads = %v", impl.payloads)
	}
	if again, _ := client.GRPC(); again != g {
		t.Error("GRPC() should return the cached connection")
	}

	// Close releases the lazily dialed connection.
	if err := client.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := g.Enqueue(ctx, &spooledgrpc.EnqueueRequest{QueueName: "q"}); err == nil {
		t.Error("the gRPC connection is still usable after Client.Close")
	}

	// WithGRPCTLS reaches the dialler: forcing TLS against a plaintext server
	// fails its handshake.
	forced, err := NewClient(WithAPIKey(testAPIKey), WithGRPCAddress(addr), WithGRPCTLS(true))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer forced.Close()
	fg, err := forced.GRPC()
	if err != nil {
		t.Fatalf("GRPC(): %v", err)
	}
	if _, err := fg.Enqueue(ctx, &spooledgrpc.EnqueueRequest{QueueName: "q"}); err == nil || !strings.Contains(err.Error(), "Unavailable") {
		t.Errorf("forced TLS against a plaintext server err = %v, want an Unavailable handshake failure", err)
	}
}

// --- Item 3 helpers: the public error surface --------------------------------

func TestPublicErrorHelpers_QuotaAndUnsupported(t *testing.T) {
	var quotaCalls, otherCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/jobs" {
			quotaCalls.Add(1)
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":"limit_exceeded","code":"QUOTA_EXCEEDED","message":"full","resource":"active_jobs","current":9,"limit":9,"plan":"free","upgrade_to":"starter"}`))
			return
		}
		otherCalls.Add(1)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	client, err := NewClient(WithAPIKey(testAPIKey), WithBaseURL(server.URL))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer client.Close()

	start := time.Now()
	_, err = client.Jobs().Create(context.Background(), &resources.CreateJobRequest{QueueName: "q", Payload: map[string]any{}})
	if !IsQuotaExceeded(err) || !IsRateLimitError(err) || IsRetryable(err) {
		t.Fatalf("err = %v: quota=%v rateLimit=%v retryable=%v", err, IsQuotaExceeded(err), IsRateLimitError(err), IsRetryable(err))
	}
	var rl *RateLimitError
	if !errors.As(err, &rl) || rl.Quota == nil || rl.Quota.Plan != "free" || rl.Quota.UpgradeTo != "starter" {
		t.Errorf("Quota details lost: %+v", rl)
	}
	// With the default retry config (1s, 2s, 4s backoff) a retried quota error would take seconds.
	if elapsed := time.Since(start); elapsed > 800*time.Millisecond || quotaCalls.Load() != 1 {
		t.Errorf("quota error took %v and %d requests; want one fast request", elapsed, quotaCalls.Load())
	}

	err = client.Ingest().GitHub(context.Background(), "org", []byte(`{}`), nil)
	if !IsUnsupportedError(err) || !errors.Is(err, ErrUnsupported) {
		t.Errorf("Ingest.GitHub err = %v, want ErrUnsupported", err)
	}
	var unsupported *UnsupportedError
	if !errors.As(err, &unsupported) {
		t.Errorf("err %T is not an *UnsupportedError", err)
	}
	if otherCalls.Load() != 0 {
		t.Errorf("unsupported call hit the network %d times", otherCalls.Load())
	}
}
