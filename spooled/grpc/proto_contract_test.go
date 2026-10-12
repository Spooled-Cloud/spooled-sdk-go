package grpc

import (
	"bytes"
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/spooled-cloud/spooled-sdk-go/spooled/grpc/pb"
)

// wireFields decodes a serialized message into field number -> varint values.
// Only varint fields are collected; that is all these tests look at.
func wireFields(t *testing.T, b []byte) map[protowire.Number][]uint64 {
	t.Helper()
	fields := map[protowire.Number][]uint64{}
	for len(b) > 0 {
		num, typ, n := protowire.ConsumeTag(b)
		if n < 0 {
			t.Fatalf("bad tag: %v", protowire.ParseError(n))
		}
		b = b[n:]
		if typ == protowire.VarintType {
			v, m := protowire.ConsumeVarint(b)
			if m < 0 {
				t.Fatalf("bad varint: %v", protowire.ParseError(m))
			}
			fields[num] = append(fields[num], v)
			b = b[m:]
			continue
		}
		m := protowire.ConsumeFieldValue(num, typ, b)
		if m < 0 {
			t.Fatalf("bad field %d: %v", num, protowire.ParseError(m))
		}
		b = b[m:]
	}
	return fields
}

// TestFailRequest_NoRetryPutsRetryFalseOnTheWire checks the serialized bytes:
// FailRequest field 4 (retry) must be present with value 0 for NoRetry, or the
// server (which reads a missing retry as "retry") never dead-letters the job.
func TestFailRequest_NoRetryPutsRetryFalseOnTheWire(t *testing.T) {
	noRetry, err := proto.Marshal(newFailRequest(&FailRequest{JobID: "j", WorkerID: "w", Error: "x", NoRetry: true, LeaseID: "l"}))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if got := wireFields(t, noRetry)[4]; len(got) != 1 || got[0] != 0 {
		t.Fatalf("NoRetry: field 4 on the wire = %v, want exactly [0] (bytes %x)", got, noRetry)
	}
	if !bytes.Contains(noRetry, []byte{0x20, 0x00}) {
		t.Errorf("NoRetry bytes %x do not contain the field-4 varint 0 (20 00)", noRetry)
	}

	retry, err := proto.Marshal(newFailRequest(&FailRequest{JobID: "j", WorkerID: "w", Error: "x", LeaseID: "l"}))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if got := wireFields(t, retry)[4]; len(got) != 1 || got[0] != 1 {
		t.Fatalf("default: field 4 on the wire = %v, want exactly [1] (bytes %x)", got, retry)
	}

	// The deprecated Retry field still changes nothing.
	legacy, err := proto.Marshal(newFailRequest(&FailRequest{JobID: "j", WorkerID: "w", Error: "x", Retry: false, LeaseID: "l"}))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if got := wireFields(t, legacy)[4]; len(got) != 1 || got[0] != 1 {
		t.Errorf("Retry:false without NoRetry: field 4 = %v, want [1] (retry stays the default)", got)
	}
}

// dispositionQueueServer answers Fail like the backend: dead-letter when retry
// is explicitly false, otherwise retry after 30 s.
type dispositionQueueServer struct {
	pb.UnimplementedQueueServiceServer
}

func (dispositionQueueServer) Fail(_ context.Context, req *pb.FailRequest) (*pb.FailResponse, error) {
	if req.Retry != nil && !*req.Retry {
		return &pb.FailResponse{Success: true, WillRetry: false}, nil
	}
	return &pb.FailResponse{Success: true, WillRetry: true, NextRetryDelaySecs: 30}, nil
}

func TestFailWithResponse_ReturnsTheServersDecision(t *testing.T) {
	listener, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	server := grpc.NewServer()
	pb.RegisterQueueServiceServer(server, dispositionQueueServer{})
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)

	client, err := NewClient(ClientOptions{Address: listener.Addr().String(), APIKey: "sp_test_key", Timeout: time.Second})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer client.Close()
	ctx := context.Background()

	resp, err := client.FailWithResponse(ctx, &FailRequest{JobID: "j", WorkerID: "w", Error: "x", LeaseID: "l"})
	if err != nil {
		t.Fatalf("FailWithResponse: %v", err)
	}
	if !resp.Success || !resp.WillRetry || resp.NextRetryDelaySecs != 30 {
		t.Errorf("default fail = %+v, want success, will retry after 30 s", resp)
	}

	resp, err = client.FailWithResponse(ctx, &FailRequest{JobID: "j", WorkerID: "w", Error: "x", LeaseID: "l", NoRetry: true})
	if err != nil {
		t.Fatalf("FailWithResponse NoRetry: %v", err)
	}
	if !resp.Success || resp.WillRetry {
		t.Errorf("NoRetry fail = %+v, want success and no retry (dead-lettered)", resp)
	}
}

// recordingWorkerServer remembers the last Register request.
type recordingWorkerServer struct {
	pb.UnimplementedWorkerServiceServer
	mu       sync.Mutex
	register *pb.RegisterWorkerRequest
}

func (s *recordingWorkerServer) Register(_ context.Context, req *pb.RegisterWorkerRequest) (*pb.RegisterWorkerResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.register = req
	id := req.GetWorkerId()
	if id == "" {
		id = "server-assigned"
	}
	return &pb.RegisterWorkerResponse{WorkerId: id, LeaseDurationSecs: 300, HeartbeatIntervalSecs: 15}, nil
}

func TestRegisterWorker_SendsStableWorkerID(t *testing.T) {
	listener, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	rec := &recordingWorkerServer{}
	server := grpc.NewServer()
	pb.RegisterWorkerServiceServer(server, rec)
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)

	client, err := NewClient(ClientOptions{Address: listener.Addr().String(), APIKey: "sp_test_key", Timeout: time.Second})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer client.Close()
	ctx := context.Background()

	resp, err := client.RegisterWorker(ctx, &RegisterWorkerRequest{QueueName: "q", Hostname: "h", WorkerID: "billing-worker-1"})
	if err != nil {
		t.Fatalf("RegisterWorker: %v", err)
	}
	if rec.register.WorkerId == nil || *rec.register.WorkerId != "billing-worker-1" {
		t.Errorf("worker_id on the wire = %v, want billing-worker-1", rec.register.WorkerId)
	}
	if resp.WorkerID != "billing-worker-1" {
		t.Errorf("WorkerID = %q, want the stable id back", resp.WorkerID)
	}

	if _, err := client.RegisterWorker(ctx, &RegisterWorkerRequest{QueueName: "q", Hostname: "h"}); err != nil {
		t.Fatalf("RegisterWorker without id: %v", err)
	}
	if rec.register.WorkerId != nil {
		t.Errorf("an empty WorkerID was sent as %q; it must stay off the wire so the server assigns one", *rec.register.WorkerId)
	}
}

// TestProto_OptionalFieldsHavePresence guards the generated code: the backend
// declares these fields `optional`, and the SDK relies on presence for them.
func TestProto_OptionalFieldsHavePresence(t *testing.T) {
	cases := []struct {
		msg   proto.Message
		field string
	}{
		{&pb.FailRequest{}, "retry"},
		{&pb.RegisterWorkerRequest{}, "worker_id"},
	}
	for _, c := range cases {
		desc := c.msg.ProtoReflect().Descriptor()
		fd := desc.Fields().ByName(protoreflect.Name(c.field))
		if fd == nil {
			t.Errorf("%s has no field %q: regenerate pb from the backend proto", desc.FullName(), c.field)
			continue
		}
		if !fd.HasPresence() {
			t.Errorf("%s.%s has no presence: the SDK proto is not the backend's `optional` field", desc.FullName(), c.field)
		}
	}
}

// TestProto_MatchesBackendProto compares the bundled proto with the backend's
// when the backend repository is checked out next to this one (the workspace
// layout scripts/generate_grpc.sh expects). It is skipped elsewhere, such as
// in this repository's own CI.
func TestProto_MatchesBackendProto(t *testing.T) {
	backend, err := os.ReadFile(filepath.Join("..", "..", "..", "spooled-backend", "proto", "spooled.proto"))
	if err != nil {
		t.Skipf("backend proto not available: %v", err)
	}
	sdk, err := os.ReadFile(filepath.Join("pb", "spooled.proto"))
	if err != nil {
		t.Fatalf("read SDK proto: %v", err)
	}
	if got, want := protoBody(string(sdk)), protoBody(string(backend)); got != want {
		t.Error("spooled/grpc/pb/spooled.proto differs from spooled-backend/proto/spooled.proto: run scripts/generate_grpc.sh")
	}
}

// protoBody drops the syntax and go_package lines (the generator adds the
// latter) and blank lines, and normalises line endings.
func protoBody(s string) string {
	var out []string
	for _, line := range strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "syntax ") || strings.HasPrefix(trimmed, "option go_package") {
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}
