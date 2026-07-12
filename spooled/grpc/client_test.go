package grpc

import (
	"context"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/test/bufconn"

	"github.com/spooled-cloud/spooled-sdk-go/spooled/grpc/pb"
)

type wireQueueServer struct {
	pb.UnimplementedQueueServiceServer
	dequeue  *pb.DequeueRequest
	complete *pb.CompleteRequest
	fail     *pb.FailRequest
	renew    *pb.RenewLeaseRequest
}

func (s *wireQueueServer) Dequeue(_ context.Context, req *pb.DequeueRequest) (*pb.DequeueResponse, error) {
	s.dequeue = req
	return &pb.DequeueResponse{Jobs: []*pb.Job{{Id: "job-1", QueueName: "q", LeaseId: "lease-from-server"}}}, nil
}

func (s *wireQueueServer) Complete(_ context.Context, req *pb.CompleteRequest) (*pb.CompleteResponse, error) {
	s.complete = req
	return &pb.CompleteResponse{Success: true}, nil
}

func (s *wireQueueServer) Fail(_ context.Context, req *pb.FailRequest) (*pb.FailResponse, error) {
	s.fail = req
	return &pb.FailResponse{Success: true}, nil
}

func (s *wireQueueServer) RenewLease(_ context.Context, req *pb.RenewLeaseRequest) (*pb.RenewLeaseResponse, error) {
	s.renew = req
	return &pb.RenewLeaseResponse{Success: true}, nil
}

func TestClient_LeaseFieldsOnGRPCWire(t *testing.T) {
	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer()
	wire := &wireQueueServer{}
	pb.RegisterQueueServiceServer(server, wire)
	go func() {
		_ = server.Serve(listener)
	}()
	defer server.Stop()
	defer listener.Close()

	useTLS := false
	client, err := NewClient(ClientOptions{
		Address: "localhost:0",
		APIKey:  "sp_test_key",
		UseTLS:  &useTLS,
		Timeout: time.Second,
		DialOptions: []grpc.DialOption{
			grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
				return listener.Dial()
			}),
		},
	})
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	defer client.Close()

	ctx := context.Background()
	dequeued, err := client.Dequeue(ctx, &DequeueRequest{
		QueueName: "q", WorkerID: "worker-1", BatchSize: 7, LeaseDurationSec: 45,
	})
	if err != nil {
		t.Fatalf("Dequeue failed: %v", err)
	}
	if wire.dequeue.GetQueueName() != "q" || wire.dequeue.GetWorkerId() != "worker-1" ||
		wire.dequeue.GetBatchSize() != 7 || wire.dequeue.GetLeaseDurationSecs() != 45 {
		t.Fatalf("unexpected dequeue wire request: %#v", wire.dequeue)
	}
	if len(dequeued.Jobs) != 1 || dequeued.Jobs[0].LeaseID != "lease-from-server" {
		t.Fatalf("dequeued lease ID not mapped from wire: %#v", dequeued.Jobs)
	}

	if err := client.Complete(ctx, &CompleteRequest{
		JobID: "job-1", WorkerID: "worker-1", LeaseID: "lease-complete", Result: map[string]any{"ok": true},
	}); err != nil {
		t.Fatalf("Complete failed: %v", err)
	}
	if wire.complete.GetJobId() != "job-1" || wire.complete.GetWorkerId() != "worker-1" ||
		wire.complete.GetLeaseId() != "lease-complete" || !wire.complete.GetResult().AsMap()["ok"].(bool) {
		t.Fatalf("unexpected complete wire request: %#v", wire.complete)
	}

	if err := client.Fail(ctx, &FailRequest{
		JobID: "job-1", WorkerID: "worker-1", LeaseID: "lease-fail", Error: "boom", Retry: true,
	}); err != nil {
		t.Fatalf("Fail failed: %v", err)
	}
	if wire.fail.GetJobId() != "job-1" || wire.fail.GetWorkerId() != "worker-1" ||
		wire.fail.GetLeaseId() != "lease-fail" || wire.fail.GetError() != "boom" || !wire.fail.GetRetry() {
		t.Fatalf("unexpected fail wire request: %#v", wire.fail)
	}

	renewed, err := client.RenewLease(ctx, &RenewLeaseRequest{
		JobID: "job-1", WorkerID: "worker-1", LeaseID: "lease-renew", ExtensionSecs: 60,
	})
	if err != nil {
		t.Fatalf("RenewLease failed: %v", err)
	}
	if !renewed.Success || wire.renew.GetJobId() != "job-1" || wire.renew.GetWorkerId() != "worker-1" ||
		wire.renew.GetLeaseId() != "lease-renew" || wire.renew.GetExtensionSecs() != 60 {
		t.Fatalf("unexpected renew wire request: %#v", wire.renew)
	}
}
