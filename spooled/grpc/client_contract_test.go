package grpc

import (
	"context"
	"math"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"

	"github.com/spooled-cloud/spooled-sdk-go/spooled/grpc/pb"
)

// recordingQueueServer remembers the last request of each kind it served.
type recordingQueueServer struct {
	pb.UnimplementedQueueServiceServer
	mu       sync.Mutex
	enqueue  *pb.EnqueueRequest
	complete *pb.CompleteRequest
	fail     *pb.FailRequest
}

func (s *recordingQueueServer) Enqueue(_ context.Context, req *pb.EnqueueRequest) (*pb.EnqueueResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.enqueue = req
	return &pb.EnqueueResponse{JobId: "job-1", Created: true}, nil
}

func (s *recordingQueueServer) Complete(_ context.Context, req *pb.CompleteRequest) (*pb.CompleteResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.complete = req
	return &pb.CompleteResponse{Success: true}, nil
}

func (s *recordingQueueServer) Fail(_ context.Context, req *pb.FailRequest) (*pb.FailResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fail = req
	return &pb.FailResponse{Success: true}, nil
}

// startPlaintextServer serves rec on a real loopback TCP port and returns
// "localhost:<port>", the address shape a developer passes for a local docker
// compose stack.
func startPlaintextServer(t *testing.T, rec *recordingQueueServer) string {
	t.Helper()
	listener, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	server := grpc.NewServer()
	pb.RegisterQueueServiceServer(server, rec)
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)

	_, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatalf("split: %v", err)
	}
	return "localhost:" + port
}

func TestDefaultUseTLS(t *testing.T) {
	cases := map[string]bool{
		"grpc.spooled.cloud:443":  true,
		"grpc.spooled.cloud":      true, // no port: gRPC dials 443
		"dns:///api.example:443":  true,
		"localhost:50051":         false,
		"127.0.0.1:50051":         false,
		"[::1]:50051":             false,
		"grpc.example.com:8443":   false, // TLS here needs an explicit UseTLS
		"passthrough:///svc:9000": false,
	}
	for addr, want := range cases {
		if got := defaultUseTLS(addr); got != want {
			t.Errorf("defaultUseTLS(%q) = %v, want %v", addr, got, want)
		}
	}
}

// TestNewClient_LocalhostIsPlaintextByDefault is the SDKGP-05 regression test:
// the localhost guard was inverted, so "localhost:PORT" was dialed with TLS and
// every RPC against a plaintext dev server failed with "first record does not
// look like a TLS handshake".
func TestNewClient_LocalhostIsPlaintextByDefault(t *testing.T) {
	rec := &recordingQueueServer{}
	addr := startPlaintextServer(t, rec)

	client, err := NewClient(ClientOptions{Address: addr, APIKey: "sp_test_key", Timeout: time.Second})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := client.Enqueue(ctx, &EnqueueRequest{QueueName: "q", Payload: map[string]any{"a": 1}}); err != nil {
		t.Fatalf("Enqueue against a plaintext localhost server failed: %v", err)
	}
}

// TestEnqueue_PayloadWithGoTypesReachesServer is the SDKGP-04 regression test:
// structpb.NewStruct rejects []string, map[string]string, time.Time and structs;
// the error was dropped and the job was created with an empty payload.
func TestEnqueue_PayloadWithGoTypesReachesServer(t *testing.T) {
	rec := &recordingQueueServer{}
	addr := startPlaintextServer(t, rec)
	client, err := NewClient(ClientOptions{Address: addr, APIKey: "sp_test_key", Timeout: time.Second})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer client.Close()

	type address struct {
		City string `json:"city"`
	}
	when := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)
	payload := map[string]any{
		"to":      "a@b.c",
		"tags":    []string{"x", "y"},
		"counts":  []int{1, 2, 3},
		"labels":  map[string]string{"env": "prod"},
		"at":      when,
		"address": address{City: "Kyiv"},
		"n":       42,
	}

	ctx := context.Background()
	if _, err := client.Enqueue(ctx, &EnqueueRequest{QueueName: "emails", Payload: payload}); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	got := rec.enqueue.GetPayload().AsMap()
	if len(got) != len(payload) {
		t.Fatalf("server payload = %v, want all %d keys (it was dropped to {})", got, len(payload))
	}
	if tags, _ := got["tags"].([]any); len(tags) != 2 || tags[0] != "x" {
		t.Errorf("tags = %v", got["tags"])
	}
	if labels, _ := got["labels"].(map[string]any); labels["env"] != "prod" {
		t.Errorf("labels = %v", got["labels"])
	}
	if got["at"] != when.Format(time.RFC3339Nano) && got["at"] != "2026-10-10T12:00:00Z" {
		t.Errorf("at = %v", got["at"])
	}
	if addr, _ := got["address"].(map[string]any); addr["city"] != "Kyiv" {
		t.Errorf("address = %v", got["address"])
	}
	if got["n"] != float64(42) {
		t.Errorf("n = %v", got["n"])
	}

	// Complete results take the same path.
	if err := client.Complete(ctx, &CompleteRequest{
		JobID: "job-1", WorkerID: "w", Result: map[string]any{"ids": []string{"a", "b"}},
	}); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if ids, _ := rec.complete.GetResult().AsMap()["ids"].([]any); len(ids) != 2 {
		t.Errorf("result = %v, want ids [a b]", rec.complete.GetResult().AsMap())
	}
}

func TestEnqueueAndComplete_UnencodableValuesReturnErrors(t *testing.T) {
	rec := &recordingQueueServer{}
	addr := startPlaintextServer(t, rec)
	client, err := NewClient(ClientOptions{Address: addr, APIKey: "sp_test_key", Timeout: time.Second})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer client.Close()
	ctx := context.Background()

	_, err = client.Enqueue(ctx, &EnqueueRequest{QueueName: "q", Payload: map[string]any{"bad": math.NaN()}})
	if err == nil || !strings.Contains(err.Error(), "payload") {
		t.Fatalf("Enqueue with NaN err = %v, want a payload error", err)
	}
	if rec.enqueue != nil {
		t.Error("Enqueue reached the server with a payload it could not encode")
	}

	err = client.Complete(ctx, &CompleteRequest{JobID: "j", WorkerID: "w", Result: map[string]any{"bad": make(chan int)}})
	if err == nil || !strings.Contains(err.Error(), "result") {
		t.Fatalf("Complete with a channel err = %v, want a result error", err)
	}
	if rec.complete != nil {
		t.Error("Complete reached the server with a result it could not encode")
	}
}

// TestFail_RetriesByDefault is the SDKGP-17 regression test: proto3 sends
// retry=false for an omitted field, and the server dead-letters on retry=false.
func TestFail_RetriesByDefault(t *testing.T) {
	rec := &recordingQueueServer{}
	addr := startPlaintextServer(t, rec)
	client, err := NewClient(ClientOptions{Address: addr, APIKey: "sp_test_key", Timeout: time.Second})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer client.Close()
	ctx := context.Background()

	if err := client.Fail(ctx, &FailRequest{JobID: "j", WorkerID: "w", Error: "boom"}); err != nil {
		t.Fatalf("Fail: %v", err)
	}
	if !rec.fail.GetRetry() {
		t.Error("Fail without Retry sent retry=false: the job would be dead-lettered on its first failure")
	}

	if err := client.Fail(ctx, &FailRequest{JobID: "j", WorkerID: "w", Error: "fatal", NoRetry: true}); err != nil {
		t.Fatalf("Fail NoRetry: %v", err)
	}
	if rec.fail.GetRetry() {
		t.Error("NoRetry must send retry=false")
	}
}

func TestFail_CoercesErrorText(t *testing.T) {
	rec := &recordingQueueServer{}
	addr := startPlaintextServer(t, rec)
	client, err := NewClient(ClientOptions{Address: addr, APIKey: "sp_test_key", Timeout: time.Second})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer client.Close()
	ctx := context.Background()

	if err := client.Fail(ctx, &FailRequest{JobID: "j", WorkerID: "w", Error: ""}); err != nil {
		t.Fatalf("Fail: %v", err)
	}
	if rec.fail.GetError() != "Job failed" {
		t.Errorf("empty error sent as %q, want %q", rec.fail.GetError(), "Job failed")
	}

	long := strings.Repeat("é", 3000) // 2 bytes per character
	if err := client.Fail(ctx, &FailRequest{JobID: "j", WorkerID: "w", Error: long}); err != nil {
		t.Fatalf("Fail: %v", err)
	}
	if n := len([]rune(rec.fail.GetError())); n != 2048 {
		t.Errorf("long error sent with %d characters, want 2048", n)
	}
}
