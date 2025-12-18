package spooled

import (
	"context"
	"os"
	"sync"
	"time"

	"github.com/spooled-cloud/spooled-sdk-go/internal/httpx"
	"github.com/spooled-cloud/spooled-sdk-go/spooled/grpc"
	"github.com/spooled-cloud/spooled-sdk-go/spooled/resources"
	"github.com/spooled-cloud/spooled-sdk-go/spooled/worker"
)

// SpooledWorkerOptions configures a Spooled worker.
type SpooledWorkerOptions struct {
	// QueueName is the name of the queue to process jobs from.
	QueueName string
	// Concurrency is the maximum number of jobs to process concurrently (default: 5).
	Concurrency int
	// PollInterval is how often to poll for new jobs (default: 1s).
	PollInterval time.Duration
	// LeaseDuration is the job lease duration in seconds (default: 30).
	LeaseDuration int
	// Hostname is the worker hostname (default: auto-detected).
	Hostname string
	// WorkerType identifies the type of worker (default: "go").
	WorkerType string
	// Version is the worker version (default: SDK version).
	Version string
	// Metadata contains additional worker metadata.
	Metadata map[string]string
	// handler is the job handler function (internal)
	handler func(context.Context, *resources.Job) (any, error)
}

// SpooledWorker is a high-level worker for processing jobs.
type SpooledWorker struct {
	jobs    *resources.JobsResource
	workers *resources.WorkersResource
	opts    SpooledWorkerOptions
	worker  *worker.Worker
}

// Start starts the worker.
func (w *SpooledWorker) Start() error {
	if w.worker != nil {
		return nil // Already started
	}

	// Set defaults
	opts := w.opts
	if opts.Concurrency == 0 {
		opts.Concurrency = 5
	}
	if opts.PollInterval == 0 {
		opts.PollInterval = time.Second
	}
	if opts.LeaseDuration == 0 {
		opts.LeaseDuration = 30
	}
	if opts.Hostname == "" {
		if hostname, err := os.Hostname(); err == nil {
			opts.Hostname = hostname
		} else {
			opts.Hostname = "unknown"
		}
	}
	if opts.WorkerType == "" {
		opts.WorkerType = "go"
	}
	if opts.Version == "" {
		opts.Version = "1.0.0"
	}
	if opts.Metadata == nil {
		opts.Metadata = make(map[string]string)
	}

	// Create low-level worker
	workerOpts := worker.Options{
		QueueName:      opts.QueueName,
		Concurrency:    opts.Concurrency,
		PollInterval:   opts.PollInterval,
		LeaseDuration:  opts.LeaseDuration,
		Hostname:       opts.Hostname,
		WorkerType:     opts.WorkerType,
		Version:        opts.Version,
		Metadata:       opts.Metadata,
	}

	w.worker = worker.NewWorker(w.jobs, w.workers, workerOpts)

	// Set handler if provided
	if w.opts.handler != nil {
		w.worker.Process(func(ctx *worker.JobContext) (map[string]any, error) {
			// Convert to simple job struct for the handler
			job := &resources.Job{
				ID:         ctx.JobID,
				QueueName:  ctx.QueueName,
				Payload:    ctx.Payload,
				RetryCount: ctx.RetryCount,
				MaxRetries: ctx.MaxRetries,
			}
			result, err := w.opts.handler(ctx.Context, job)
			if resultMap, ok := result.(map[string]any); ok {
				return resultMap, err
			}
			// If result is not a map, wrap it
			return map[string]any{"result": result}, err
		})
	}

	return w.worker.Start(context.Background())
}

// Stop stops the worker.
func (w *SpooledWorker) Stop() error {
	if w.worker == nil {
		return nil
	}
	return w.worker.Stop()
}

// Process registers a job handler function.
func (w *SpooledWorker) Process(handler func(context.Context, *resources.Job) (any, error)) {
	if w.worker != nil {
		panic("cannot set handler after starting worker")
	}
	// Store handler for when worker is created
	w.opts.handler = handler
}

// Client is the main Spooled SDK client.
type Client struct {
	cfg       *Config
	transport *httpx.Transport
	mu        sync.RWMutex
	closed    bool

	// Resource accessors
	jobs          *resources.JobsResource
	queues        *resources.QueuesResource
	workers       *resources.WorkersResource
	schedules     *resources.SchedulesResource
	workflows     *resources.WorkflowsResource
	webhooks      *resources.WebhooksResource
	organizations *resources.OrganizationsResource
	apiKeys       *resources.APIKeysResource
	billing       *resources.BillingResource
	dashboard     *resources.DashboardResource
	health        *resources.HealthResource
	metrics       *resources.MetricsResource
	auth          *resources.AuthResource
	admin         *resources.AdminResource
	ingest        *resources.IngestResource

	// Lazy-loaded clients
	grpcClient *grpc.Client
}

// NewClient creates a new Spooled client with the given options.
func NewClient(opts ...Option) (*Client, error) {
	cfg := resolveConfig(opts...)

	// Validate configuration
	if cfg.APIKey == "" && cfg.AccessToken == "" {
		return nil, ErrNoAuth
	}
	if cfg.APIKey != "" {
		if err := ValidateAPIKey(cfg.APIKey); err != nil {
			return nil, err
		}
	}

	// Create transport
	transport := httpx.NewTransport(httpx.Config{
		BaseURL:          cfg.BaseURL,
		APIKey:           cfg.APIKey,
		AccessToken:      cfg.AccessToken,
		RefreshToken:     cfg.RefreshToken,
		AdminKey:         cfg.AdminKey,
		UserAgent:        cfg.UserAgent,
		Headers:          cfg.Headers,
		Timeout:          cfg.Timeout,
		AutoRefreshToken: cfg.AutoRefreshToken,
		Retry: httpx.RetryConfig{
			MaxRetries: cfg.Retry.MaxRetries,
			BaseDelay:  cfg.Retry.BaseDelay,
			MaxDelay:   cfg.Retry.MaxDelay,
			Factor:     cfg.Retry.Factor,
			Jitter:     cfg.Retry.Jitter,
		},
		CircuitBreaker: httpx.CircuitBreakerConfig{
			Enabled:          cfg.CircuitBreaker.Enabled,
			FailureThreshold: cfg.CircuitBreaker.FailureThreshold,
			SuccessThreshold: cfg.CircuitBreaker.SuccessThreshold,
			Timeout:          cfg.CircuitBreaker.Timeout,
		},
		Logger: wrapLogger(cfg.Logger),
	})

	c := &Client{
		cfg:       cfg,
		transport: transport,
	}

	// Initialize resources
	c.initResources()

	return c, nil
}

// wrapLogger wraps a spooled.Logger to an httpx.Logger.
func wrapLogger(l Logger) httpx.Logger {
	if l == nil {
		return nil
	}
	return &loggerWrapper{l}
}

type loggerWrapper struct {
	Logger
}

func (w *loggerWrapper) Debug(msg string, keysAndValues ...any) {
	w.Logger.Debug(msg, keysAndValues...)
}

// initResources initializes all resource accessors.
func (c *Client) initResources() {
	c.jobs = resources.NewJobsResource(c.transport)
	c.queues = resources.NewQueuesResource(c.transport)
	c.workers = resources.NewWorkersResource(c.transport)
	c.schedules = resources.NewSchedulesResource(c.transport)
	c.workflows = resources.NewWorkflowsResource(c.transport)
	c.webhooks = resources.NewWebhooksResource(c.transport)
	c.organizations = resources.NewOrganizationsResource(c.transport)
	c.apiKeys = resources.NewAPIKeysResource(c.transport)
	c.billing = resources.NewBillingResource(c.transport)
	c.dashboard = resources.NewDashboardResource(c.transport)
	c.health = resources.NewHealthResource(c.transport)
	c.metrics = resources.NewMetricsResource(c.transport)
	c.auth = resources.NewAuthResource(c.transport)
	c.admin = resources.NewAdminResource(c.transport)
	c.ingest = resources.NewIngestResource(c.transport)
}

// Close closes the client and releases any resources.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	return nil
}

// GetConfig returns a copy of the client configuration.
func (c *Client) GetConfig() Config {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return *c.cfg
}

// Jobs returns the Jobs resource.
func (c *Client) Jobs() *resources.JobsResource {
	return c.jobs
}

// Queues returns the Queues resource.
func (c *Client) Queues() *resources.QueuesResource {
	return c.queues
}

// Workers returns the Workers resource.
func (c *Client) Workers() *resources.WorkersResource {
	return c.workers
}

// Schedules returns the Schedules resource.
func (c *Client) Schedules() *resources.SchedulesResource {
	return c.schedules
}

// Workflows returns the Workflows resource.
func (c *Client) Workflows() *resources.WorkflowsResource {
	return c.workflows
}

// Webhooks returns the Webhooks resource.
func (c *Client) Webhooks() *resources.WebhooksResource {
	return c.webhooks
}

// Organizations returns the Organizations resource.
func (c *Client) Organizations() *resources.OrganizationsResource {
	return c.organizations
}

// APIKeys returns the API Keys resource.
func (c *Client) APIKeys() *resources.APIKeysResource {
	return c.apiKeys
}

// Billing returns the Billing resource.
func (c *Client) Billing() *resources.BillingResource {
	return c.billing
}

// Dashboard returns the Dashboard resource.
func (c *Client) Dashboard() *resources.DashboardResource {
	return c.dashboard
}

// Health returns the Health resource.
func (c *Client) Health() *resources.HealthResource {
	return c.health
}

// Metrics returns the Metrics resource.
func (c *Client) Metrics() *resources.MetricsResource {
	return c.metrics
}

// Auth returns the Auth resource.
func (c *Client) Auth() *resources.AuthResource {
	return c.auth
}

// Admin returns the Admin resource.
func (c *Client) Admin() *resources.AdminResource {
	return c.admin
}

// Ingest returns the Ingest resource.
func (c *Client) Ingest() *resources.IngestResource {
	return c.ingest
}

// GRPC returns the gRPC client for high-performance operations.
//
// Note: This method dials the gRPC server the first time it is called.
// Callers should handle connection errors (e.g., local dev without gRPC running).
func (c *Client) GRPC() (*grpc.Client, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.grpcClient != nil {
		return c.grpcClient, nil
	}

	grpcClient, err := grpc.NewClient(grpc.ClientOptions{
		Address: c.cfg.GRPCAddress,
		APIKey:  c.cfg.APIKey,
		Timeout: 5 * time.Second,
	})
	if err != nil {
		return nil, err
	}

	c.grpcClient = grpcClient
	return c.grpcClient, nil
}

// Realtime returns a realtime client for WebSocket/SSE event streaming.
// TODO: Implement realtime client
// func (c *Client) Realtime(opts ...realtime.Option) *realtime.Client {
// 	return realtime.NewClient(realtime.Config{
// 		BaseURL: c.cfg.BaseURL,
// 		APIKey:  c.cfg.APIKey,
// 	}, opts...)
// }

// log logs a debug message if logging is enabled.
func (c *Client) log(msg string, keysAndValues ...any) {
	if c.cfg.Logger != nil {
		c.cfg.Logger.Debug(msg, keysAndValues...)
	}
}

// NewSpooledWorker creates a new Spooled worker for processing jobs.
//
// Example:
//
//	w := spooled.NewSpooledWorker(client, spooled.SpooledWorkerOptions{
//		QueueName:   "emails",
//		Concurrency: 10,
//	})
//	defer w.Stop()
//
//	w.Process(func(ctx context.Context, job *resources.Job) (any, error) {
//		// Process job
//		return map[string]any{"processed": true}, nil
//	})
//
//	if err := w.Start(); err != nil {
//		log.Fatal(err)
//	}
func NewSpooledWorker(c *Client, opts SpooledWorkerOptions) *SpooledWorker {
	return &SpooledWorker{
		jobs:    c.Jobs(),
		workers: c.Workers(),
		opts:    opts,
	}
}

// Timestamp helpers
func now() time.Time {
	return time.Now().UTC()
}

func parseTime(s string) (time.Time, error) {
	return time.Parse(time.RFC3339, s)
}

func formatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}
