// Package grpc provides a gRPC client for the Spooled API.
package grpc

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/spooled-cloud/spooled-sdk-go/spooled/grpc/pb"
	"github.com/spooled-cloud/spooled-sdk-go/spooled/resources"
)

// Client is the gRPC client for Spooled.
type Client struct {
	conn         *grpc.ClientConn
	queueClient  pb.QueueServiceClient
	workerClient pb.WorkerServiceClient
	apiKey       string
}

// ClientOptions configures the gRPC client.
type ClientOptions struct {
	// Address is the gRPC server address (e.g., "grpc.spooled.cloud:443")
	Address string
	// APIKey is the API key for authentication
	APIKey string
	// UseTLS forces TLS on or off. When nil, TLS is used for port 443 (and for an
	// address with no port, which gRPC dials on 443) and plaintext is used for
	// every other port, including localhost:50051. Set it to dial a TLS
	// endpoint on another port, such as ":8443".
	UseTLS *bool
	// TLSConfig is custom TLS configuration (optional)
	TLSConfig *tls.Config
	// DialOptions are additional gRPC dial options
	DialOptions []grpc.DialOption
	// Timeout is the connection timeout
	Timeout time.Duration
}

// DefaultAddress is the default gRPC server address.
const DefaultAddress = "grpc.spooled.cloud:443"

// NewClient creates a new gRPC client.
func NewClient(opts ClientOptions) (*Client, error) {
	if opts.Address == "" {
		opts.Address = DefaultAddress
	}
	if opts.Timeout == 0 {
		opts.Timeout = 30 * time.Second
	}

	// Determine TLS setting
	useTLS := defaultUseTLS(opts.Address)
	if opts.UseTLS != nil {
		useTLS = *opts.UseTLS
	}

	dialOpts := []grpc.DialOption{}

	// Add TLS credentials
	if useTLS {
		tlsConfig := opts.TLSConfig
		if tlsConfig == nil {
			tlsConfig = &tls.Config{
				MinVersion: tls.VersionTLS12,
			}
		}
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)))
	} else {
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	// Add custom dial options
	dialOpts = append(dialOpts, opts.DialOptions...)

	// Create connection with timeout
	ctx, cancel := context.WithTimeout(context.Background(), opts.Timeout)
	defer cancel()

	conn, err := grpc.DialContext(ctx, opts.Address, dialOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to gRPC server: %w", err)
	}

	return &Client{
		conn:         conn,
		queueClient:  pb.NewQueueServiceClient(conn),
		workerClient: pb.NewWorkerServiceClient(conn),
		// gRPC metadata (like HTTP headers) rejects '\r' / '\n' at the transport
		// layer with an opaque error. Trim here so callers who construct
		// `ClientOptions` directly (bypassing spooled.resolveConfig) don't get
		// an inscrutable failure from a stray newline in their API key.
		apiKey: strings.TrimSpace(opts.APIKey),
	}, nil
}

// defaultUseTLS decides whether to use TLS when the caller did not say. Only the
// standard TLS port, or no port at all (gRPC then dials 443), gets TLS; a
// development server on any other port, "localhost:50051" included, is plaintext.
//
// The earlier check meant to skip TLS for localhost but had the condition
// inverted, so "localhost:50051" was dialed with TLS and failed its first RPC
// with "first record does not look like a TLS handshake".
func defaultUseTLS(address string) bool {
	target := address
	// Strip a resolver scheme such as "dns:///host:443".
	if i := strings.LastIndex(target, "/"); i >= 0 {
		target = target[i+1:]
	}
	_, port, err := net.SplitHostPort(target)
	if err != nil {
		// No port (or an unparsable one): gRPC dials 443.
		return true
	}
	return port == "443"
}

// Close closes the gRPC connection.
func (c *Client) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// withAuth adds authentication metadata to the context.
func (c *Client) withAuth(ctx context.Context) context.Context {
	if c.apiKey != "" {
		return metadata.AppendToOutgoingContext(ctx, "x-api-key", c.apiKey)
	}
	return ctx
}

// Queue Service Methods

// EnqueueRequest is the request for enqueueing a job.
//
// MaxRetries and TimeoutSeconds are optional pointers:
//   - nil → field omitted on the wire; backend applies QUEUE_DEFAULT_*
//   - non-nil → value sent (backend still maps <=0 to QUEUE_DEFAULT_*)
//
// Explicit zero retries cannot be expressed over gRPC proto3 int32 (same as
// REST omit vs 0 distinction on the server).
type EnqueueRequest struct {
	QueueName      string
	Payload        map[string]any
	Priority       int32
	MaxRetries     *int32
	TimeoutSeconds *int32
	ScheduledAt    *time.Time
	IdempotencyKey string
}

// Int32 returns a pointer to v, for the optional *int32 fields on
// EnqueueRequest (e.g. MaxRetries: grpc.Int32(3)).
func Int32(v int32) *int32 {
	return &v
}

// EnqueueResponse is the response from enqueueing a job.
type EnqueueResponse struct {
	JobID   string
	Created bool
}

// Enqueue enqueues a new job.
func (c *Client) Enqueue(ctx context.Context, req *EnqueueRequest) (*EnqueueResponse, error) {
	ctx = c.withAuth(ctx)

	pbReq := &pb.EnqueueRequest{
		QueueName:      req.QueueName,
		Priority:       req.Priority,
		IdempotencyKey: req.IdempotencyKey,
	}
	if req.MaxRetries != nil {
		pbReq.MaxRetries = *req.MaxRetries
	}
	if req.TimeoutSeconds != nil {
		pbReq.TimeoutSeconds = *req.TimeoutSeconds
	}

	// Convert payload to protobuf Struct. An error here must reach the caller: the
	// server treats a missing payload as {}, so swallowing it created the job with
	// an empty payload and reported success.
	payload, err := toStruct(req.Payload)
	if err != nil {
		return nil, fmt.Errorf("invalid payload: %w", err)
	}
	pbReq.Payload = payload

	// Convert scheduled time
	if req.ScheduledAt != nil {
		pbReq.ScheduledAt = timestamppb.New(*req.ScheduledAt)
	}

	resp, err := c.queueClient.Enqueue(ctx, pbReq)
	if err != nil {
		return nil, err
	}

	return &EnqueueResponse{
		JobID:   resp.JobId,
		Created: resp.Created,
	}, nil
}

// DequeueRequest is the request for dequeuing jobs.
type DequeueRequest struct {
	QueueName        string
	WorkerID         string
	BatchSize        int32
	LeaseDurationSec int32
}

// Job represents a dequeued job.
type Job struct {
	ID             string
	QueueName      string
	Payload        map[string]any
	Priority       int32
	RetryCount     int32
	MaxRetries     int32
	TimeoutSeconds int32
	LeaseExpiresAt *time.Time
	// LeaseID is the lease fencing token issued by the server for this
	// dequeue. Echo it back on Complete/Fail/RenewLease so the operation
	// applies only to the lease this worker actually holds (empty = legacy
	// worker_id fence).
	LeaseID string
}

// DequeueResponse is the response from dequeuing jobs.
type DequeueResponse struct {
	Jobs []*Job
}

// Dequeue dequeues jobs for a worker.
func (c *Client) Dequeue(ctx context.Context, req *DequeueRequest) (*DequeueResponse, error) {
	ctx = c.withAuth(ctx)

	pbReq := &pb.DequeueRequest{
		QueueName:         req.QueueName,
		WorkerId:          req.WorkerID,
		BatchSize:         req.BatchSize,
		LeaseDurationSecs: req.LeaseDurationSec,
	}

	resp, err := c.queueClient.Dequeue(ctx, pbReq)
	if err != nil {
		return nil, err
	}

	jobs := make([]*Job, len(resp.Jobs))
	for i, j := range resp.Jobs {
		jobs[i] = pbJobToJob(j)
	}

	return &DequeueResponse{Jobs: jobs}, nil
}

// CompleteRequest is the request to complete a job.
type CompleteRequest struct {
	JobID    string
	WorkerID string
	Result   map[string]any
	// LeaseID is the lease fencing token from the dequeued Job. When set, the
	// completion succeeds only if it matches the job's current lease.
	LeaseID string
}

// Complete marks a job as completed.
func (c *Client) Complete(ctx context.Context, req *CompleteRequest) error {
	ctx = c.withAuth(ctx)

	pbReq := &pb.CompleteRequest{
		JobId:    req.JobID,
		WorkerId: req.WorkerID,
		LeaseId:  req.LeaseID,
	}

	result, err := toStruct(req.Result)
	if err != nil {
		return fmt.Errorf("invalid result: %w", err)
	}
	pbReq.Result = result

	_, err = c.queueClient.Complete(ctx, pbReq)
	return err
}

// FailRequest is the request to fail a job.
type FailRequest struct {
	JobID    string
	WorkerID string
	Error    string
	// Retry has no effect any more: a failed job is retried, up to the job's
	// max_retries, unless NoRetry is set.
	//
	// Deprecated: the zero value used to mean "dead-letter now" because proto3
	// cannot tell an omitted bool from false, so a Fail without Retry skipped
	// every retry. Retrying is now the default; set NoRetry to opt out.
	Retry bool
	// NoRetry dead-letters the job immediately instead of scheduling a retry.
	NoRetry bool
	// LeaseID is the lease fencing token from the dequeued Job. When set, the
	// failure succeeds only if it matches the job's current lease.
	LeaseID string
}

// Fail marks a job as failed. Unless req.NoRetry is set the job is retried with
// backoff until max_retries is used up, like a failure reported over REST.
func (c *Client) Fail(ctx context.Context, req *FailRequest) error {
	ctx = c.withAuth(ctx)

	pbReq := &pb.FailRequest{
		JobId:    req.JobID,
		WorkerId: req.WorkerID,
		// The server rejects (REST) or has to trim (gRPC) an empty or over-long
		// message; send one it accepts.
		Error:   resources.CoerceFailureMessage(req.Error),
		Retry:   !req.NoRetry,
		LeaseId: req.LeaseID,
	}

	_, err := c.queueClient.Fail(ctx, pbReq)
	return err
}

// RenewLeaseRequest is the request to renew a job lease.
type RenewLeaseRequest struct {
	JobID         string
	WorkerID      string
	ExtensionSecs int32
	// LeaseID is the lease fencing token from the dequeued Job. When set, the
	// renewal succeeds only if it matches the job's current lease.
	LeaseID string
}

// RenewLeaseResponse is the response from renewing a lease.
type RenewLeaseResponse struct {
	Success      bool
	NewExpiresAt *time.Time
}

// RenewLease renews the lease on a job.
func (c *Client) RenewLease(ctx context.Context, req *RenewLeaseRequest) (*RenewLeaseResponse, error) {
	ctx = c.withAuth(ctx)

	pbReq := &pb.RenewLeaseRequest{
		JobId:         req.JobID,
		WorkerId:      req.WorkerID,
		ExtensionSecs: req.ExtensionSecs,
		LeaseId:       req.LeaseID,
	}

	resp, err := c.queueClient.RenewLease(ctx, pbReq)
	if err != nil {
		return nil, err
	}

	result := &RenewLeaseResponse{
		Success: resp.Success,
	}
	if resp.NewExpiresAt != nil {
		t := resp.NewExpiresAt.AsTime()
		result.NewExpiresAt = &t
	}

	return result, nil
}

// GetJob retrieves a job by ID.
func (c *Client) GetJob(ctx context.Context, jobID string) (*Job, error) {
	ctx = c.withAuth(ctx)

	resp, err := c.queueClient.GetJob(ctx, &pb.GetJobRequest{JobId: jobID})
	if err != nil {
		return nil, err
	}

	return pbJobToJob(resp.Job), nil
}

// QueueStats represents queue statistics.
type QueueStats struct {
	QueueName  string
	Pending    int64
	Scheduled  int64
	Processing int64
	Completed  int64
	Failed     int64
	Deadletter int64
	Total      int64
	MaxAgeMs   int64
}

// GetQueueStats retrieves queue statistics.
func (c *Client) GetQueueStats(ctx context.Context, queueName string) (*QueueStats, error) {
	ctx = c.withAuth(ctx)

	resp, err := c.queueClient.GetQueueStats(ctx, &pb.GetQueueStatsRequest{QueueName: queueName})
	if err != nil {
		return nil, err
	}

	return &QueueStats{
		QueueName:  resp.QueueName,
		Pending:    resp.Pending,
		Scheduled:  resp.Scheduled,
		Processing: resp.Processing,
		Completed:  resp.Completed,
		Failed:     resp.Failed,
		Deadletter: resp.Deadletter,
		Total:      resp.Total,
		MaxAgeMs:   resp.MaxAgeMs,
	}, nil
}

// StreamJobs opens a streaming connection to receive jobs.
func (c *Client) StreamJobs(ctx context.Context, queueName, workerID string) (pb.QueueService_StreamJobsClient, error) {
	ctx = c.withAuth(ctx)

	return c.queueClient.StreamJobs(ctx, &pb.StreamJobsRequest{
		QueueName: queueName,
		WorkerId:  workerID,
	})
}

// ProcessJobs opens a bidirectional stream for job processing.
func (c *Client) ProcessJobs(ctx context.Context) (pb.QueueService_ProcessJobsClient, error) {
	ctx = c.withAuth(ctx)
	return c.queueClient.ProcessJobs(ctx)
}

// Worker Service Methods

// RegisterWorkerRequest is the request to register a worker.
type RegisterWorkerRequest struct {
	QueueName      string
	Hostname       string
	MaxConcurrency int32
	Version        string
	Metadata       map[string]string
}

// RegisterWorkerResponse is the response from registering a worker.
type RegisterWorkerResponse struct {
	WorkerID             string
	HeartbeatIntervalSec int32
	LeaseDurationSec     int32
}

// RegisterWorker registers a new worker.
func (c *Client) RegisterWorker(ctx context.Context, req *RegisterWorkerRequest) (*RegisterWorkerResponse, error) {
	ctx = c.withAuth(ctx)

	pbReq := &pb.RegisterWorkerRequest{
		QueueName:      req.QueueName,
		Hostname:       req.Hostname,
		MaxConcurrency: req.MaxConcurrency,
		Version:        req.Version,
		Metadata:       req.Metadata,
	}

	resp, err := c.workerClient.Register(ctx, pbReq)
	if err != nil {
		return nil, err
	}

	return &RegisterWorkerResponse{
		WorkerID:             resp.WorkerId,
		HeartbeatIntervalSec: resp.HeartbeatIntervalSecs,
		LeaseDurationSec:     resp.LeaseDurationSecs,
	}, nil
}

// WorkerHeartbeatRequest is the request for a worker heartbeat.
type WorkerHeartbeatRequest struct {
	WorkerID    string
	CurrentJobs int32
	Status      string
}

// WorkerHeartbeat sends a worker heartbeat.
func (c *Client) WorkerHeartbeat(ctx context.Context, req *WorkerHeartbeatRequest) error {
	ctx = c.withAuth(ctx)

	_, err := c.workerClient.Heartbeat(ctx, &pb.HeartbeatRequest{
		WorkerId:    req.WorkerID,
		CurrentJobs: req.CurrentJobs,
		Status:      req.Status,
	})
	return err
}

// DeregisterWorker deregisters a worker.
func (c *Client) DeregisterWorker(ctx context.Context, workerID string) error {
	ctx = c.withAuth(ctx)

	_, err := c.workerClient.Deregister(ctx, &pb.DeregisterRequest{
		WorkerId: workerID,
	})
	return err
}

// Helper functions

// toStruct converts a Go map to a protobuf Struct through JSON.
//
// structpb.NewStruct accepts only the exact dynamic types the protobuf JSON
// mapping uses (map[string]any, []any, string, float64, bool, nil), so a
// []string, a map[string]string, a time.Time or a struct anywhere in the map was
// rejected. Encoding to JSON first accepts everything encoding/json can encode.
// protobuf Struct numbers are doubles, so integers above 2^53 lose precision
// over gRPC; use REST for those.
func toStruct(m map[string]any) (*structpb.Struct, error) {
	if m == nil {
		return nil, nil
	}
	data, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	s := &structpb.Struct{}
	if err := s.UnmarshalJSON(data); err != nil {
		return nil, err
	}
	return s, nil
}

func pbJobToJob(j *pb.Job) *Job {
	if j == nil {
		return nil
	}

	job := &Job{
		ID:             j.Id,
		QueueName:      j.QueueName,
		Priority:       j.Priority,
		RetryCount:     j.RetryCount,
		MaxRetries:     j.MaxRetries,
		TimeoutSeconds: j.TimeoutSeconds,
		LeaseID:        j.LeaseId,
	}

	if j.Payload != nil {
		job.Payload = j.Payload.AsMap()
	}

	if j.LeaseExpiresAt != nil {
		t := j.LeaseExpiresAt.AsTime()
		job.LeaseExpiresAt = &t
	}

	return job
}
