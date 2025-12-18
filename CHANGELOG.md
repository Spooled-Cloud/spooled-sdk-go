# Changelog

All notable changes to the Spooled Go SDK will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [1.0.0] - 2024-12-18

### Added

- **Core SDK**
  - `spooled.NewClient()` with functional options pattern
  - Full REST API coverage for all Spooled endpoints
  - Type-safe request/response structures
  - Context propagation for all operations

- **Jobs**
  - Create, get, list, cancel jobs
  - Bulk enqueue operations
  - Job priority and scheduling
  - Idempotency key support
  - Retry configuration
  - Dead Letter Queue (DLQ) management

- **Workers**
  - Worker registration and heartbeat
  - High-level `Worker` runtime with job processing
  - Concurrent job processing with configurable concurrency
  - Graceful shutdown support
  - Progress reporting and logging

- **Queues**
  - List and get queue details
  - Queue statistics (pending, processing, completed, failed)
  - Pause/resume queues
  - Queue configuration updates

- **Workflows**
  - Create workflows with DAG dependencies
  - Fan-out/fan-in patterns
  - Workflow status tracking
  - Cancel and retry workflows

- **Schedules**
  - Cron-based job scheduling
  - Timezone support
  - Pause/resume schedules
  - Manual trigger execution
  - Execution history

- **Webhooks**
  - Outgoing webhook configuration
  - Event filtering by queue
  - Webhook testing
  - Delivery history and retries

- **Real-time Events**
  - WebSocket client for bidirectional events
  - SSE client for server-sent events
  - Automatic reconnection with backoff
  - Event filtering and subscriptions

- **gRPC**
  - High-performance gRPC client
  - Enqueue/dequeue operations
  - Worker registration via gRPC
  - Queue statistics

- **Authentication**
  - API key authentication
  - JWT token support with automatic refresh
  - Admin key support for admin endpoints

- **Reliability**
  - Automatic retries with exponential backoff
  - Circuit breaker for fault tolerance
  - Configurable timeouts
  - Request ID tracking

- **Error Handling**
  - Typed errors (NotFoundError, ValidationError, etc.)
  - Error inspection helpers (IsRetryableError, etc.)
  - Rate limit handling with retry-after

### Documentation

- Comprehensive README with examples
- Package-level godoc documentation
- Runnable examples for all major features
- Integration test suite

## [1.0.3] - 2024-12-18

### Fixed

- Fixed all golangci-lint errors for clean CI builds
- Removed unused code (functions, variables, imports)
- Fixed code formatting across all files (gofmt)
- Removed deprecated `rand.Seed` call (auto-seeded in Go 1.20+)
- Updated linter exclusions for test files, scripts, and deprecation warnings

## [1.0.2] - 2024-12-18

### Fixed

- Changed API key prefix from `sk_live_`/`sk_test_` to `sp_live_`/`sp_test_` to avoid GitHub secret scanning false positives
- Removed deprecated `check-shadowing` option from golangci-lint config (v1.64+ compatibility)

## [Unreleased]

### Planned

- Batch operations optimization
- Streaming job results
- Enhanced metrics integration
- OpenTelemetry tracing support

[1.0.3]: https://github.com/spooled-cloud/spooled-sdk-go/releases/tag/v1.0.3
[1.0.2]: https://github.com/spooled-cloud/spooled-sdk-go/releases/tag/v1.0.2
[1.0.0]: https://github.com/spooled-cloud/spooled-sdk-go/releases/tag/v1.0.0
[Unreleased]: https://github.com/spooled-cloud/spooled-sdk-go/compare/v1.0.3...HEAD

