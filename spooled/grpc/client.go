// Package grpc provides a gRPC client for the Spooled API.
package grpc

import (
	"context"
	"crypto/tls"
	"fmt"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/spooled-cloud/spooled-sdk-go/spooled/grpc/pb"
)

// Client is the gRPC client for Spooled.
type Client struct {
	conn          *grpc.ClientConn
	queueClient   pb.QueueServiceClient
	workerClient  pb.WorkerServiceClient
	apiKey        string
}

// ClientOptions configures the gRPC client.
type ClientOptions struct {
	// Address is the gRPC server address (e.g., "grpc.spooled.cloud:443")
	Address string
	// APIKey is the API key for authentication
	APIKey string
	// UseTLS enables TLS (default: true for port 443)
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
	useTLS := true
	if opts.UseTLS != nil {
		useTLS = *opts.UseTLS
	} else if !strings.HasSuffix(opts.Address, ":443") && !strings.Contains(opts.Address, "localhost") {
		// Default to TLS for :443, no TLS for localhost
		useTLS = strings.HasSuffix(opts.Address, ":443")
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
		apiKey:       opts.APIKey,
	}, nil
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
type EnqueueRequest struct {
	QueueName      string
	Payload        map[string]any
	Priority       int32
	MaxRetries     int32
	TimeoutSeconds int32
	ScheduledAt    *time.Time
	IdempotencyKey string
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
		MaxRetries:     req.MaxRetries,
		TimeoutSeconds: req.TimeoutSeconds,
		IdempotencyKey: req.IdempotencyKey,
	}

	// Convert payload to protobuf Struct
	if req.Payload != nil {
		s, err := structpb.NewStruct(req.Payload)
		if err == nil {
			pbReq.Payload = s
		}
	}

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
}

// Complete marks a job as completed.
func (c *Client) Complete(ctx context.Context, req *CompleteRequest) error {
	ctx = c.withAuth(ctx)

	pbReq := &pb.CompleteRequest{
		JobId:    req.JobID,
		WorkerId: req.WorkerID,
	}

	if req.Result != nil {
		s, err := structpb.NewStruct(req.Result)
		if err == nil {
			pbReq.Result = s
		}
	}

	_, err := c.queueClient.Complete(ctx, pbReq)
	return err
}

// FailRequest is the request to fail a job.
type FailRequest struct {
	JobID    string
	WorkerID string
	Error    string
	Retry    bool
}

// Fail marks a job as failed.
func (c *Client) Fail(ctx context.Context, req *FailRequest) error {
	ctx = c.withAuth(ctx)

	pbReq := &pb.FailRequest{
		JobId:    req.JobID,
		WorkerId: req.WorkerID,
		Error:    req.Error,
		Retry:    req.Retry,
	}

	_, err := c.queueClient.Fail(ctx, pbReq)
	return err
}

// RenewLeaseRequest is the request to renew a job lease.
type RenewLeaseRequest struct {
	JobID         string
	WorkerID      string
	ExtensionSecs int32
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
