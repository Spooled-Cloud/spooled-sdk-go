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
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"

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

// backendSchema is spooled.proto as the backend declares it: the SDK's
// descriptor with FailRequest.retry turned back into an `optional bool`. The
// SDK keeps a plain bool there (scripts/generate_grpc.sh), so pb cannot tell
// "retry omitted" from "retry false"; a message decoded with this schema can,
// as the backend does.
func backendSchema(t *testing.T) protoreflect.FileDescriptor {
	t.Helper()
	fdp := protodesc.ToFileDescriptorProto(pb.File_spooled_proto)
	found := false
	for _, msg := range fdp.GetMessageType() {
		if msg.GetName() != "FailRequest" {
			continue
		}
		for _, field := range msg.GetField() {
			if field.GetName() != "retry" {
				continue
			}
			field.Proto3Optional = proto.Bool(true)
			field.OneofIndex = proto.Int32(int32(len(msg.GetOneofDecl())))
			msg.OneofDecl = append(msg.OneofDecl, &descriptorpb.OneofDescriptorProto{Name: proto.String("_retry")})
			found = true
		}
	}
	if !found {
		t.Fatal("FailRequest.retry is not in the SDK descriptor")
	}
	fd, err := protodesc.NewFile(fdp, protoregistry.GlobalFiles)
	if err != nil {
		t.Fatalf("build the backend schema: %v", err)
	}
	return fd
}

// retryOf reports whether a FailRequest decoded with backendSchema carried
// retry, and its value.
func retryOf(m protoreflect.Message) (sent, value bool) {
	fd := m.Descriptor().Fields().ByName("retry")
	return m.Has(fd), m.Get(fd).Bool()
}

// backendRetry decodes a serialized FailRequest as the backend does.
func backendRetry(t *testing.T, b []byte) (sent, value bool) {
	t.Helper()
	msg := dynamicpb.NewMessage(backendSchema(t).Messages().ByName("FailRequest"))
	if err := proto.Unmarshal(b, msg); err != nil {
		t.Fatalf("decode as the backend: %v", err)
	}
	return retryOf(msg)
}

// TestPB_FailRequestRetryStaysABool keeps pb source compatible with 1.4.0: a
// ProcessJobs user builds pb.FailRequest with `Retry: true` and reads Retry as
// a bool. Generating pb from the backend's `optional bool retry` as it is turns
// Retry into a *bool, and this file stops compiling.
func TestPB_FailRequestRetryStaysABool(t *testing.T) {
	req := &pb.ProcessRequest{Request: &pb.ProcessRequest_Fail{Fail: &pb.FailRequest{
		JobId: "j", WorkerId: "w", Error: "x", Retry: true, LeaseId: "l",
	}}}
	if f := req.GetFail(); !f.Retry || !f.GetRetry() {
		t.Error("Retry: true did not stick")
	}
	fd := (&pb.FailRequest{}).ProtoReflect().Descriptor().Fields().ByName("retry")
	if fd.HasPresence() {
		t.Error("pb FailRequest.retry has presence: pb was generated without the SDK override in scripts/generate_grpc.sh, so pb.FailRequest.Retry is no longer a bool")
	}
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
	if sent, retry := backendRetry(t, noRetry); !sent || retry {
		t.Errorf("NoRetry: the backend reads retry sent=%t value=%t, want an explicit false", sent, retry)
	}

	retry, err := proto.Marshal(newFailRequest(&FailRequest{JobID: "j", WorkerID: "w", Error: "x", LeaseID: "l"}))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if got := wireFields(t, retry)[4]; len(got) != 1 || got[0] != 1 {
		t.Fatalf("default: field 4 on the wire = %v, want exactly [1] (bytes %x)", got, retry)
	}
	if sent, value := backendRetry(t, retry); !sent || !value {
		t.Errorf("default: the backend reads retry sent=%t value=%t, want an explicit true", sent, value)
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

// TestSetNoRetry covers the helper for code that builds pb.FailRequest itself,
// such as a ProcessJobs stream user.
func TestSetNoRetry(t *testing.T) {
	fail := &pb.FailRequest{JobId: "j", WorkerId: "w", Error: "x", Retry: true, LeaseId: "l"}
	if got := SetNoRetry(fail); got != fail {
		t.Fatal("SetNoRetry must return its argument")
	}
	SetNoRetry(fail) // a second call changes nothing
	if fail.Retry {
		t.Error("SetNoRetry left Retry true")
	}
	b, err := proto.Marshal(fail)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if got := wireFields(t, b)[4]; len(got) != 1 || got[0] != 0 {
		t.Fatalf("field 4 on the wire = %v, want exactly [0] (bytes %x)", got, b)
	}
	if sent, retry := backendRetry(t, b); !sent || retry {
		t.Errorf("the backend reads retry sent=%t value=%t, want an explicit false", sent, retry)
	}

	// Nested in a ProcessRequest, as the stream sends it.
	stream, err := proto.Marshal(&pb.ProcessRequest{Request: &pb.ProcessRequest_Fail{Fail: fail}})
	if err != nil {
		t.Fatalf("marshal ProcessRequest: %v", err)
	}
	msg := dynamicpb.NewMessage(backendSchema(t).Messages().ByName("ProcessRequest"))
	if err := proto.Unmarshal(stream, msg); err != nil {
		t.Fatalf("decode ProcessRequest as the backend: %v", err)
	}
	nested := msg.Get(msg.Descriptor().Fields().ByName("fail")).Message()
	if sent, retry := retryOf(nested); !sent || retry {
		t.Errorf("ProcessRequest.fail: the backend reads retry sent=%t value=%t, want an explicit false", sent, retry)
	}

	// Other unknown fields are kept.
	other := &pb.FailRequest{JobId: "j"}
	other.ProtoReflect().SetUnknown(protowire.AppendVarint(protowire.AppendTag(nil, 99, protowire.VarintType), 7))
	b, err = proto.Marshal(SetNoRetry(other))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	fields := wireFields(t, b)
	if got := fields[99]; len(got) != 1 || got[0] != 7 {
		t.Errorf("unknown field 99 = %v, want [7] kept", got)
	}
	if got := fields[4]; len(got) != 1 || got[0] != 0 {
		t.Errorf("field 4 = %v, want [0]", got)
	}

	// Without SetNoRetry a plain false never reaches the server, which then
	// retries the job: the reason the helper exists.
	plain, err := proto.Marshal(&pb.FailRequest{JobId: "j", Retry: false, LeaseId: "l"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if sent, _ := backendRetry(t, plain); sent {
		t.Error("a plain Retry: false was sent; update the SetNoRetry docs")
	}

	if SetNoRetry(nil) != nil {
		t.Error("SetNoRetry(nil) must return nil")
	}
}

// TestSetNoRetry_IsOneWay pins what the SetNoRetry doc comment says: there is no
// undoing it. Setting Retry afterwards writes the known field 4 and the
// unknown field 4 = 0 SetNoRetry added, two occurrences, and the unknown field
// is written last, so the last value wins and the server still reads false.
func TestSetNoRetry_IsOneWay(t *testing.T) {
	fail := SetNoRetry(&pb.FailRequest{JobId: "j", WorkerId: "w", Error: "x", LeaseId: "l"})
	fail.Retry = true
	b, err := proto.Marshal(fail)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if got := wireFields(t, b)[4]; len(got) != 2 || got[0] != 1 || got[1] != 0 {
		t.Fatalf("field 4 on the wire = %v, want [1 0]: the known field, then the unknown one (bytes %x)", got, b)
	}
	if sent, retry := backendRetry(t, b); !sent || retry {
		t.Errorf("the backend reads retry sent=%t value=%t, want an explicit false (last value wins)", sent, retry)
	}

	// A second SetNoRetry puts it back to a single field 4 = 0.
	b, err = proto.Marshal(SetNoRetry(fail))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if got := wireFields(t, b)[4]; len(got) != 1 || got[0] != 0 {
		t.Errorf("after a second SetNoRetry, field 4 = %v, want exactly [0] (bytes %x)", got, b)
	}
}

// backendFailServer serves QueueService.Fail the way the backend decides it:
// retry omitted or true -> retried after 30 s, retry false -> dead-lettered. It
// decodes with backendSchema, so it sees what the backend sees.
type backendFailServer struct {
	addr  string
	typ   protoreflect.MessageType
	mu    sync.Mutex
	sent  bool
	retry bool
}

func startBackendFailServer(t *testing.T) *backendFailServer {
	t.Helper()
	s := &backendFailServer{typ: dynamicpb.NewMessageType(backendSchema(t).Messages().ByName("FailRequest"))}
	listener, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	server := grpc.NewServer()
	server.RegisterService(&grpc.ServiceDesc{
		ServiceName: pb.QueueService_ServiceDesc.ServiceName,
		HandlerType: (*any)(nil),
		Methods:     []grpc.MethodDesc{{MethodName: "Fail", Handler: s.fail}},
		Metadata:    pb.QueueService_ServiceDesc.Metadata,
	}, s)
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)
	s.addr = listener.Addr().String()
	return s
}

func (s *backendFailServer) fail(_ any, _ context.Context, dec func(any) error, _ grpc.UnaryServerInterceptor) (any, error) {
	req := s.typ.New()
	if err := dec(req.Interface()); err != nil {
		return nil, err
	}
	sent, retry := retryOf(req)
	s.mu.Lock()
	s.sent, s.retry = sent, retry
	s.mu.Unlock()
	if sent && !retry {
		return &pb.FailResponse{Success: true, WillRetry: false}, nil
	}
	return &pb.FailResponse{Success: true, WillRetry: true, NextRetryDelaySecs: 30}, nil
}

// lastRetry is what the last Fail carried: whether retry was sent, and its value.
func (s *backendFailServer) lastRetry() (sent, retry bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sent, s.retry
}

func TestFailWithResponse_ReturnsTheServersDecision(t *testing.T) {
	srv := startBackendFailServer(t)
	client, err := NewClient(ClientOptions{Address: srv.addr, APIKey: "sp_test_key", Timeout: time.Second})
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
// FailRequest.retry is the one exception (sdkProtoOverrides).
func TestProto_OptionalFieldsHavePresence(t *testing.T) {
	cases := []struct {
		msg   proto.Message
		field string
	}{
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

// sdkProtoOverrides are the deliberate differences between the bundled proto
// and the backend's, line for line, as scripts/generate_grpc.sh writes them.
//
// FailRequest.retry: the backend's `optional bool` would make
// pb.FailRequest.Retry a *bool and break code that builds pb.FailRequest (a
// ProcessJobs stream user, a test server). The SDK keeps a plain bool and sends
// an explicit false with SetNoRetry.
var sdkProtoOverrides = []struct{ backend, sdk string }{
	{
		backend: "    optional bool retry = 4;",
		sdk:     "    bool retry = 4; // SDK copy: a plain bool, so false is not sent and means retry; grpc.SetNoRetry sends false.",
	},
}

// TestProto_GeneratorAppliesTheOverrides checks that scripts/generate_grpc.sh
// writes exactly the overrides TestProto_MatchesBackendProto accepts, and that
// the bundled proto has each of them once.
func TestProto_GeneratorAppliesTheOverrides(t *testing.T) {
	script, err := os.ReadFile(filepath.Join("..", "..", "scripts", "generate_grpc.sh"))
	if err != nil {
		t.Fatalf("read generator: %v", err)
	}
	sdk, err := os.ReadFile(filepath.Join("pb", "spooled.proto"))
	if err != nil {
		t.Fatalf("read SDK proto: %v", err)
	}
	lines := strings.Split(strings.ReplaceAll(string(sdk), "\r\n", "\n"), "\n")
	for _, o := range sdkProtoOverrides {
		if !strings.Contains(string(script), "'"+o.backend+"'") || !strings.Contains(string(script), "'"+o.sdk+"'") {
			t.Errorf("scripts/generate_grpc.sh does not rewrite %q to %q", o.backend, o.sdk)
		}
		if n := countLines(lines, o.sdk); n != 1 {
			t.Errorf("pb/spooled.proto has %q %d times, want once: run scripts/generate_grpc.sh", o.sdk, n)
		}
		if n := countLines(lines, o.backend); n != 0 {
			t.Errorf("pb/spooled.proto has the backend line %q: run scripts/generate_grpc.sh", o.backend)
		}
	}
}

func countLines(lines []string, want string) int {
	n := 0
	for _, line := range lines {
		if line == want {
			n++
		}
	}
	return n
}

// TestProto_MatchesBackendProto compares the bundled proto with the backend's
// when the backend repository is checked out next to this one (the workspace
// layout scripts/generate_grpc.sh expects). It is skipped elsewhere, such as
// in this repository's own CI. sdkProtoOverrides are the only differences it
// accepts.
func TestProto_MatchesBackendProto(t *testing.T) {
	backend, err := os.ReadFile(filepath.Join("..", "..", "..", "spooled-backend", "proto", "spooled.proto"))
	if err != nil {
		t.Skipf("backend proto not available: %v", err)
	}
	sdk, err := os.ReadFile(filepath.Join("pb", "spooled.proto"))
	if err != nil {
		t.Fatalf("read SDK proto: %v", err)
	}
	sdkBody := protoBody(string(sdk))
	for _, o := range sdkProtoOverrides {
		sdkBody = replaceLine(sdkBody, o.sdk, o.backend)
	}
	if got, want := sdkBody, protoBody(string(backend)); got != want {
		t.Error("spooled/grpc/pb/spooled.proto differs from spooled-backend/proto/spooled.proto beyond sdkProtoOverrides: run scripts/generate_grpc.sh")
	}
}

// replaceLine replaces each line equal to from with to.
func replaceLine(body, from, to string) string {
	lines := strings.Split(body, "\n")
	for i, line := range lines {
		if line == from {
			lines[i] = to
		}
	}
	return strings.Join(lines, "\n")
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
