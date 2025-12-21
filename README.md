# Spooled Go SDK

Official Go SDK for [Spooled Cloud](https://spooled.cloud) — a modern, scalable job queue and task scheduler.

[**Live Demo (SpriteForge)**](https://example.spooled.cloud) • [Documentation](https://spooled.cloud/docs)

[![Go Reference](https://pkg.go.dev/badge/github.com/spooled-cloud/spooled-sdk-go.svg)](https://pkg.go.dev/github.com/spooled-cloud/spooled-sdk-go)
[![Go Report Card](https://goreportcard.com/badge/github.com/spooled-cloud/spooled-sdk-go)](https://goreportcard.com/report/github.com/spooled-cloud/spooled-sdk-go)
[![License](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)

## Features

- **Idiomatic Go API** with functional options pattern
- **Full API Coverage** — Access all Spooled API endpoints  
- **Type Safety** — Strongly typed requests and responses
- **Context Support** — Full `context.Context` propagation
- **Automatic Retries** — Exponential backoff with jitter
- **Circuit Breaker** — Fault tolerance for unreliable networks
- **Worker Runtime** — Process jobs with concurrent workers
- **Real-time Events** — WebSocket and SSE support
- **gRPC Support** — High-performance streaming client
- **Workflow DAGs** — Complex job dependencies
- **Automatic JWT Refresh** — Single-flight token refresh

## Installation

```bash
go get github.com/spooled-cloud/spooled-sdk-go
```

**Requirements:** Go 1.22 or later

## Quick Start

### Create a Job

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/spooled-cloud/spooled-sdk-go/spooled"
    "github.com/spooled-cloud/spooled-sdk-go/spooled/resources"
)

func main() {
    // Create client
    client, err := spooled.NewClient(
        spooled.WithAPIKey("sp_live_your_api_key"),
        // For self-hosted: spooled.WithBaseURL("https://your-server.com"),
    )
    if err != nil {
        log.Fatal(err)
    }

    ctx := context.Background()

    // Create a job
    priority := 5
    maxRetries := 3
    result, err := client.Jobs().Create(ctx, &resources.CreateJobRequest{
        QueueName: "email-notifications",
        Payload: map[string]any{
            "to":      "user@example.com",
            "subject": "Welcome!",
            "body":    "Thanks for signing up.",
        },
        Priority:   &priority,
        MaxRetries: &maxRetries,
    })
    if err != nil {
        log.Fatal(err)
    }

    fmt.Printf("Created job: %s\n", result.ID)

    // Get job status
    job, err := client.Jobs().Get(ctx, result.ID)
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("Status: %s\n", job.Status)
}
```

### Process Jobs with a Worker

```go
package main

import (
    "context"
    "fmt"
    "log"
    "os"
    "os/signal"
    "syscall"

    "github.com/spooled-cloud/spooled-sdk-go/spooled"
    "github.com/spooled-cloud/spooled-sdk-go/spooled/worker"
)

func main() {
    client, err := spooled.NewClient(
        spooled.WithAPIKey("sp_live_your_api_key"),
    )
    if err != nil {
        log.Fatal(err)
    }

    // Create worker
    w := worker.NewWorker(client.Jobs(), client.Workers(), worker.Options{
        QueueName:   "email-notifications",
        Concurrency: 10,
    })

    // Register job handler
    w.Process(func(ctx *worker.JobContext) (map[string]any, error) {
        fmt.Printf("Processing job %s\n", ctx.JobID)
        
        // Access payload
        to := ctx.Payload["to"].(string)
        subject := ctx.Payload["subject"].(string)
        
        // Do your work here
        fmt.Printf("Sending email to %s: %s\n", to, subject)
        
        // Report progress
        ctx.Progress(50, "Email sent")
        
        return map[string]any{"sent": true}, nil
    })

    // Handle events
    w.OnEvent(func(event worker.Event) {
        fmt.Printf("Event: %s\n", event.Type)
    })

    // Start worker
    ctx, cancel := context.WithCancel(context.Background())
    if err := w.Start(ctx); err != nil {
        log.Fatal(err)
    }

    // Graceful shutdown
    sigCh := make(chan os.Signal, 1)
    signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
    <-sigCh

    cancel()
    w.Stop()
}
```

## Real-world examples (beginner friendly)

If you want 5 copy/paste “real life” setups (Stripe → jobs, GitHub Actions → jobs, cron schedules, CSV import, website signup), see:

- `https://github.com/spooled-cloud/spooled-backend/blob/main/docs/guides/real-world-examples.md`

## Documentation

| Guide | Description |
|-------|-------------|
| [Getting Started](#quick-start) | Installation, setup, and first job |
| [Configuration](#configuration) | All configuration options |
| [Workers](#workers) | Job processing with Worker |
| [Workflows](#workflows-dags) | Building DAGs with job dependencies |
| [gRPC](#grpc-high-performance) | High-performance streaming |
| [Real-time Events](#real-time-events) | WebSocket and SSE |
| [Error Handling](#error-handling) | Typed errors and retries |

## Examples

See the [`examples/`](examples/) directory for runnable code:

| Example | Description |
|---------|-------------|
| [`quick-start/`](examples/quick-start/) | Basic SDK usage |
| [`worker/`](examples/worker/) | Processing jobs with Worker |
| [`workflow/`](examples/workflow/) | Complex workflows with dependencies |
| [`grpc/`](examples/grpc/) | High-performance gRPC streaming |
| [`realtime/`](examples/realtime/) | Real-time event streaming |
| [`schedules/`](examples/schedules/) | Cron schedules |
| [`webhooks/`](examples/webhooks/) | Webhook configuration |
| [`api-keys/`](examples/api-keys/) | API key management |
| [`bulk-operations/`](examples/bulk-operations/) | Bulk job operations |
| [`error-handling/`](examples/error-handling/) | Error handling patterns |

Run any example with:

```bash
API_KEY=sp_live_... go run examples/quick-start/main.go
```

## Configuration

### Client Options

```go
client, err := spooled.NewClient(
    // Authentication (required - one of these)
    spooled.WithAPIKey("sp_live_..."),
    spooled.WithAccessToken("eyJ..."),
    
    // Endpoints (optional)
    spooled.WithBaseURL("https://api.spooled.cloud"),
    spooled.WithWSURL("wss://api.spooled.cloud/api/v1/ws"),
    spooled.WithGRPCAddress("grpc.spooled.cloud:443"),
    
    // Timeouts (optional)
    spooled.WithTimeout(30 * time.Second),
    
    // Retries (optional)
    spooled.WithRetryConfig(spooled.RetryConfig{
        MaxRetries: 3,
        BaseDelay:  100 * time.Millisecond,
        MaxDelay:   5 * time.Second,
    }),
    
    // Circuit breaker (optional)
    spooled.WithCircuitBreaker(spooled.CircuitBreakerConfig{
        Enabled:          true,
        FailureThreshold: 5,
        Timeout:          30 * time.Second,
    }),
    
    // Debug logging (optional)
    spooled.WithDebug(true),
)
```

### Environment Variables

```bash
SPOOLED_API_KEY=sp_live_...
SPOOLED_BASE_URL=https://api.spooled.cloud
SPOOLED_GRPC_ADDRESS=grpc.spooled.cloud:443
```

## Core Concepts

### Jobs

Jobs are units of work with payloads, priorities, and retry policies:

```go
priority := 5
maxRetries := 3
timeout := 300
scheduledAt := time.Now().Add(time.Hour)

result, err := client.Jobs().Create(ctx, &resources.CreateJobRequest{
    QueueName:      "my-queue",
    Payload:        map[string]any{"data": "value"},
    Priority:       &priority,        // -100 to 100
    MaxRetries:     &maxRetries,
    TimeoutSeconds: &timeout,
    ScheduledAt:    &scheduledAt,
    IdempotencyKey: ptr("unique-key"),
})

// List jobs
jobs, err := client.Jobs().List(ctx, &resources.ListJobsParams{
    QueueName: ptr("my-queue"),
    Status:    ptr(resources.JobStatusPending),
    Tag:       ptr("billing"), // Optional: filter by a single tag
    Limit:     ptr(10),
})

// Cancel a job
err = client.Jobs().Cancel(ctx, jobID)
```

### Workers

Process jobs with the built-in worker runtime:

```go
w := worker.NewWorker(client.Jobs(), client.Workers(), worker.Options{
    QueueName:         "my-queue",
    Concurrency:       10,              // Max concurrent jobs
    PollInterval:      time.Second,     // Polling frequency
    LeaseDuration:     30,              // Lease duration in seconds
    HeartbeatFraction: 0.5,             // Heartbeat at 50% of lease
    ShutdownTimeout:   30 * time.Second,
})

w.Process(func(ctx *worker.JobContext) (map[string]any, error) {
    // ctx.Context - Go context with cancellation
    // ctx.JobID - Unique job ID
    // ctx.QueueName - Queue name
    // ctx.Payload - Job payload
    // ctx.RetryCount - Current retry attempt
    // ctx.MaxRetries - Maximum retries
    // ctx.Progress(percent, message) - Report progress
    // ctx.Log(level, message, meta) - Log messages
    
    return map[string]any{"result": "success"}, nil
})

// Event handlers
w.OnEvent(func(event worker.Event) {
    switch event.Type {
    case worker.EventJobCompleted:
        data := event.Data.(worker.JobCompletedData)
        fmt.Printf("Job %s completed in %v\n", data.JobID, data.Duration)
    case worker.EventJobFailed:
        data := event.Data.(worker.JobFailedData)
        fmt.Printf("Job %s failed: %v\n", data.JobID, data.Error)
    }
})

ctx := context.Background()
w.Start(ctx)

// Graceful shutdown
w.Stop()
```

### Workflows (DAGs)

Orchestrate multiple jobs with dependencies:

```go
workflow, err := client.Workflows().Create(ctx, &resources.CreateWorkflowRequest{
    Name: "ETL Pipeline",
    Jobs: []resources.WorkflowJob{
        {
            Key:       "extract",
            QueueName: "etl",
            Payload:   map[string]any{"step": "extract"},
        },
        {
            Key:       "transform",
            QueueName: "etl",
            Payload:   map[string]any{"step": "transform"},
            DependsOn: []string{"extract"},
        },
        {
            Key:       "load",
            QueueName: "etl",
            Payload:   map[string]any{"step": "load"},
            DependsOn: []string{"transform"},
        },
    },
})

// Get workflow status
wf, err := client.Workflows().Get(ctx, workflow.WorkflowID)
fmt.Printf("Status: %s\n", wf.Status)
```

### Schedules

Run jobs on a cron schedule:

```go
schedule, err := client.Schedules().Create(ctx, &resources.CreateScheduleRequest{
    Name:           "Daily Report",
    CronExpression: "0 9 * * *",       // 9 AM daily (5-field cron)
    Timezone:       ptr("America/New_York"),
    QueueName:      "reports",
    PayloadTemplate: map[string]any{"type": "daily"},
})

fmt.Printf("Next run: %v\n", schedule.NextRunAt)

// Pause/resume
client.Schedules().Pause(ctx, schedule.ID)
client.Schedules().Resume(ctx, schedule.ID)

// Trigger immediately
client.Schedules().Trigger(ctx, schedule.ID)
```

### Queues

Manage queues and view statistics:

```go
// List queues
queues, err := client.Queues().List(ctx)

// Get queue stats
stats, err := client.Queues().GetStats(ctx, "my-queue")
fmt.Printf("Pending: %d, Processing: %d\n", stats.Pending, stats.Processing)

// Pause/resume queue
client.Queues().Pause(ctx, "my-queue", nil)
client.Queues().Resume(ctx, "my-queue")

// Purge all pending jobs
client.Queues().Purge(ctx, "my-queue")
```

### Real-time Events

Subscribe to real-time job events via WebSocket or SSE:

```go
// WebSocket client
ws := realtime.NewWebSocketClient(realtime.ConnectionOptions{
    WSURL:  "wss://api.spooled.cloud/api/v1/ws",
    Token:  "your-jwt-token",
    APIKey: "sp_live_...",
})

ws.OnJobEvent(realtime.EventJobCompleted, func(event *realtime.JobEvent) {
    fmt.Printf("Job completed: %s\n", event.JobID)
})

ws.OnStateChange(func(state realtime.ConnectionState) {
    fmt.Printf("Connection state: %s\n", state)
})

ws.Connect()
ws.Subscribe(realtime.SubscriptionFilter{QueueName: "my-queue"})

// SSE client (unidirectional)
sse := realtime.NewSSEClient(realtime.ConnectionOptions{
    BaseURL: "https://api.spooled.cloud",
    APIKey:  "sp_live_...",
})

sse.OnEvent(func(event *realtime.Event) {
    fmt.Printf("Event: %s\n", event.Type)
})

sse.ConnectWithFilter(&realtime.SubscriptionFilter{
    QueueName: "my-queue",
})
```

### gRPC (High Performance)

Use gRPC for high-throughput scenarios:

```go
import "github.com/spooled-cloud/spooled-sdk-go/spooled/grpc"

client, err := grpc.NewClient(grpc.ClientOptions{
    Address: "grpc.spooled.cloud:443",
    APIKey:  "sp_live_...",
})
defer client.Close()

// Enqueue a job
resp, err := client.Enqueue(ctx, &grpc.EnqueueRequest{
    QueueName: "high-throughput",
    Payload:   map[string]any{"data": "value"},
    Priority:  5,
})
fmt.Printf("Job ID: %s\n", resp.JobID)

// Dequeue jobs
jobs, err := client.Dequeue(ctx, &grpc.DequeueRequest{
    QueueName: "high-throughput",
    WorkerID:  "worker-1",
    BatchSize: 10,
})

// Stream jobs (server-side streaming)
stream, err := client.StreamJobs(ctx, "high-throughput", "worker-1")
for {
    job, err := stream.Recv()
    if err == io.EOF {
        break
    }
    // Process job...
}
```

### Webhooks

Configure outgoing webhooks for job events:

```go
// Create webhook
webhook, err := client.Webhooks().Create(ctx, &resources.CreateWebhookRequest{
    URL:       "https://your-app.com/webhooks/spooled",
    Events:    []string{"job.completed", "job.failed"},
    QueueName: ptr("my-queue"),
    Secret:    ptr("whsec_..."),
})

// Test webhook
client.Webhooks().Test(ctx, webhook.ID)

// List deliveries
deliveries, err := client.Webhooks().ListDeliveries(ctx, webhook.ID, nil)
```

### Organizations

Manage your organization and track usage:

```go
// List organizations
orgs, err := client.Organizations().List(ctx)

// Get organization details
org, err := client.Organizations().Get(ctx, orgID)

// Get usage information
usage, err := client.Organizations().Usage(ctx, orgID)
fmt.Printf("Jobs today: %d/%d\n", usage.Usage.JobsToday.Current, *usage.Limits.MaxJobsPerDay)
```

### API Keys

Manage API keys:

```go
// List API keys
keys, err := client.APIKeys().List(ctx)

// Create new API key
key, err := client.APIKeys().Create(ctx, &resources.CreateAPIKeyRequest{
    Name: "Production Key",
})
fmt.Printf("Key: %s (save this, it won't be shown again)\n", key.Key)

// Revoke API key
client.APIKeys().Revoke(ctx, keyID)
```

## Error Handling

The SDK provides typed errors for different failure scenarios:

```go
import "github.com/spooled-cloud/spooled-sdk-go/spooled"

job, err := client.Jobs().Get(ctx, "invalid-id")
if err != nil {
    switch e := err.(type) {
    case *spooled.NotFoundError:
        fmt.Printf("Job not found: %s\n", e.Message)
    case *spooled.AuthenticationError:
        fmt.Printf("Invalid API key: %s\n", e.Message)
    case *spooled.RateLimitError:
        fmt.Printf("Rate limited. Retry after: %v\n", e.Reset)
    case *spooled.ValidationError:
        fmt.Printf("Invalid request: %v\n", e.Errors)
    default:
        fmt.Printf("Error: %v\n", err)
    }
}

// Check error types
if spooled.IsRetryableError(err) {
    // Safe to retry
}
if spooled.IsNetworkError(err) {
    // Network issue
}
if spooled.IsTimeoutError(err) {
    // Request timed out
}
```

## Development

### Using Make

```bash
# Build, lint, and test
make

# Run specific targets
make build          # Build the SDK
make test           # Run unit tests
make test-coverage  # Tests with coverage report
make lint           # Run linter
make fmt            # Format code
make examples       # Build all examples

# Integration tests (requires API_KEY)
API_KEY=sp_live_... make integration-test

# See all commands
make help
```

### Testing

```bash
# Unit tests
go test ./...

# With race detection
go test -race ./...

# With coverage
go test -cover ./...

# Integration tests (requires API key)
API_KEY=sp_live_... go run scripts/test-local/main.go

# Full production test suite (all features)
API_KEY=sp_live_... \
ADMIN_KEY=your_admin_key \
BASE_URL=https://api.spooled.cloud \
GRPC_ADDRESS=grpc.spooled.cloud:443 \
SKIP_GRPC=0 \
SKIP_STRESS=0 \
go run scripts/test-local/main.go
```

## Contributing

Contributions are welcome! Please see [CONTRIBUTING.md](https://github.com/spooled-cloud/spooled-backend/blob/main/CONTRIBUTING.md).

## License

Apache 2.0 - see [LICENSE](LICENSE) for details.

## Support

- 📖 [Documentation](https://spooled.cloud/docs)
- 🐛 [Issue Tracker](https://github.com/spooled-cloud/spooled-sdk-go/issues)
- 📧 [Email Support](mailto:support@spooled.cloud)
