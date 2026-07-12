# Changelog

All notable changes to the Spooled Go SDK will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [1.0.20] - 2026-07-12

### Fixed

- **Worker execution identity is immutable for the lifetime of each lease.**
  Heartbeat, completion, failure, cancellation, and cleanup now use the exact
  execution captured at claim time. An older execution of the same job cannot
  borrow a replacement lease token or delete the replacement from active work.
- **Lease expiry cancels handlers immediately.** A typed `LEASE_EXPIRED`
  heartbeat response cancels that exact handler context, stops renewal, and
  emits `job:lease_lost` with the rejected operation and lease identity.
- **Settlement events reflect backend confirmation.** `job:completed` and
  `job:failed` are emitted only after the backend accepts settlement. An
  execution canceled by lease loss skips settlement after its handler returns,
  avoiding stale requests and duplicate events. Rejected settlement emits
  `job:lease_lost` for lease conflicts or `worker:error` for other errors, and
  confirmed failure events populate `WillRetry` from the backend response or
  claim-time retry state when that response field is unavailable.
- Added same-job/two-lease ordering, stale-heartbeat cancellation, rejected
  settlement, and in-process gRPC wire-field regression tests.

## [1.0.19] - 2026-07-11

### Added

- **Lease fencing token support** (backend v0.1.94, audit F9). Claim/dequeue
  responses now carry a `lease_id` fencing token, and the SDK echoes it back on
  complete/fail/heartbeat/renew so an operation succeeds only if the worker
  still holds the job's current lease (a stale token is rejected with 409
  `LEASE_EXPIRED` over REST, `FAILED_PRECONDITION` over gRPC). REST:
  `ClaimedJob.LeaseID` plus optional `LeaseID` on `CompleteJobRequest`,
  `FailJobRequest`, `HeartbeatRequest`, and `RenewLeaseRequest`; the polling
  worker captures the token at claim time and sends it automatically. gRPC:
  regenerated stubs with the new `lease_id` fields; the wrapper `Job` exposes
  `LeaseID`, and `CompleteRequest`/`FailRequest`/`RenewLeaseRequest` accept it.
  Omitting the token preserves the legacy worker_id-only fence, so existing
  code keeps working unchanged.

## [1.0.18] - 2026-07-09

### Changed

- No functional/library changes over 1.0.17. Test-only lint fix (`noctx`): the
  credential-trim test now uses `http.NewRequestWithContext`. Published as a new
  version because pkg.go.dev tags are immutable (a re-tag of 1.0.17 is not
  possible), so the CI-green tree ships as 1.0.18.

## [1.0.17] - 2026-07-09

### Fixed

- **Real-time events now dispatch to typed handlers.** The server tags events
  with PascalCase variant names (`JobCreated`, `JobStatusChange`, `QueueStats`,
  `WorkerHeartbeat`, …), but the SDK only recognized dotted names, so typed
  `OnJobEvent`/`OnQueueEvent`/`OnWorkerEvent` handlers never fired (only the
  catch-all `OnEvent` did). Incoming event types are now normalized to the SDK's
  canonical dotted names (`job.created`, `job.status_changed`, `queue.stats`,
  `worker.heartbeat`, `worker.registered`, `worker.deregistered`,
  `system.health`, `ping`, `error`) on both the WebSocket and SSE transports
  before dispatch. Unknown/future types still pass through to `OnEvent`.
- **WebSocket subscribe/unsubscribe no longer stall.** The client sent
  `{"type":"subscribe",...}` and then blocked up to 10s waiting for a
  `"subscribed"` acknowledgement the server never sends — hanging every reconnect
  resubscribe. It now sends the server's expected
  `{"cmd":"subscribe","queue","job_id"}` (and the `unsubscribe` equivalent)
  fire-and-forget, recording the filter for replay on reconnect without waiting
  on a reply.
- **Validation errors with array `details` are parsed correctly.** A 400 body of
  the backend's shape `{"code":"VALIDATION_ERROR","details":[{"field":…}]}` has
  `details` as an ARRAY. The decoder previously typed `details` as an object, so
  the whole JSON decode failed and silently dropped `Code`, `Message`, and
  `Details`. The decoder now tolerates `details` being either an array or an
  object: an object is used as-is and an array is exposed under
  `err.Details["errors"]`, with `Code`/`Message` populated either way.
  `spooled.IsValidationError` and `spooled.AsSpooledError` work as expected.

## [1.0.16] - 2026-07-09

### Fixed

- **Credentials are trimmed of surrounding whitespace.** API keys, access tokens,
  and refresh tokens read from a file or environment variable often carry a
  trailing newline; the client now trims them at config resolution (an
  all-whitespace value is treated as unset). Prevents a cryptic failure such as
  Go's `net/http: invalid header field value` on a newline-tainted key.

## [1.0.15] - 2026-07-08

### Fixed

- **Realtime no longer logs in on every WebSocket (re)connect.** The WebSocket
  client exchanged its API key for a JWT via `POST /api/v1/auth/login` on each
  connect and reconnect, so a reconnect storm could hammer the rate-limited
  login endpoint into a 429 and prevent realtime from recovering. The JWT is
  now cached together with its decoded `exp` claim and reused across
  reconnects; the client only re-logs in when no token is cached, the cached
  token is at or near expiry (60s leeway), or the server rejects it (401/403 on
  the WebSocket upgrade). Concurrent reconnects coalesce into a single login
  instead of stampeding the endpoint. A statically configured access token is
  still used verbatim without any login.

## [1.0.14] - 2026-07-08

### Fixed

- **Typed error helpers now match the errors the client actually returns.** The
  public `spooled.*Error` types were a parallel hierarchy the client never
  produced, so `spooled.IsNotFoundError`, `IsValidationError`,
  `IsAuthenticationError`, `IsRateLimitError`, `IsSpooledError`, and
  `AsSpooledError` always reported no match against real API errors (only the
  interface-based `IsRetryable` worked). The public error types are now aliases
  for the concrete types the transport returns, so every `Is*Error` helper,
  `AsSpooledError`, and the documented `errors.As(err, &spooled.RateLimitError{})`
  pattern classify real 4xx/5xx responses correctly.

### Added

- `spooled.IsAuthorizationError` (403), `spooled.IsConflictError` (409),
  `spooled.IsPayloadTooLargeError` (413), and `spooled.IsServerError` (5xx)
  classifiers to round out the public error API.

### Changed

- `examples/error-handling` no longer imports the unreachable `internal/httpx`
  package; it now demonstrates error classification using only the public
  `spooled` API.

## [1.0.13] - 2026-07-08

### Fixed

- **Realtime WebSocket authentication.** The client sent auth in an
  `Authorization`/`X-API-Key` header the `/ws` endpoint ignores, so realtime never
  connected. It now exchanges an API key for a JWT via `POST /api/v1/auth/login`
  and dials `?token=<jwt>`.
- **SSE** authenticates with the `?api_key=` query param (the backend does not read
  `X-API-Key`).
- **Worker heartbeat** sends a valid status (`healthy`/`draining`/`offline`) instead
  of the rejected `active`/`stopping`.
- **Data race** on the access token: reads/writes are now guarded by a mutex.
- **Circuit breaker** no longer counts non-retryable 4xx as failures and records one
  outcome per call instead of one per retry attempt; half-open probes are bounded.
- **429** handling honors the `Retry-After` header.
- **`WithRetry`** no longer resets the other retry fields when only `MaxRetries` is set.
- **Reconnect** backoff can no longer integer-overflow to zero, and a scheduled
  reconnect no longer fires after `Disconnect()`.

## [1.0.12] - 2026-07-07

### Changed

- Migrated the WebSocket dependency from the deprecated `nhooyr.io/websocket`
  to the maintained `coder/websocket` fork.
- The default worker version now reports the SDK release (`1.0.12`) instead of
  a hardcoded `1.0.0`.

### Fixed

- Corrected the API key prefix in the config comment (`sp_`, not `sk_`).

## [1.0.0] - 2025-01-18

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

## [1.0.3] - 2025-01-18

### Fixed

- Fixed all golangci-lint errors for clean CI builds
- Removed unused code (functions, variables, imports)
- Fixed code formatting across all files (gofmt)
- Removed deprecated `rand.Seed` call (auto-seeded in Go 1.20+)
- Updated linter exclusions for test files, scripts, and deprecation warnings

## [1.0.2] - 2025-01-18

### Fixed

- SDK validation now accepts both `sk_live_`/`sk_test_` (production keys) and `sp_live_`/`sp_test_` (for documentation examples that avoid GitHub secret scanning)
- Removed deprecated `check-shadowing` option from golangci-lint config (v1.64+ compatibility)

## [1.0.11] - 2026-05-05

### Fixed

- CI workflow: scope coverage to packages with tests via `-coverpkg`,
  drop Go 1.24 from matrix to match the module's minimum Go version
  (avoids `covdata` toolchain error during `go test -coverprofile`)

### Retracted

- v1.0.10 — proxy.golang.org locked the version to a transient commit
  during the release dance; the canonical content lives in v1.0.11

## [1.0.10] - 2026-05-05 [RETRACTED]

### Added

- Client `Realtime()` method returning `RealtimeClient` (WebSocket)
- Client `RealtimeSSE()` method returning `*SSEClient`
- Auth helper preferring access token over API key for realtime connections
- `normalizeRealtimeWSURL` for smart WebSocket URL resolution

## [1.0.9] - 2025-12-21

### Changed

- Added Live Demo (SpriteForge) link to README

## [1.0.8] - 2025-12-20

### Added

- Tag filtering support for job listing

## [1.0.7] - 2025-12-19

### Fixed

- Removed trailing newlines from CI, linter, changelog, contributing, and security files

## [1.0.6] - 2025-12-18

### Changed

- Linked backend real-world examples from documentation

## [1.0.4] - 2025-12-18

### Fixed

- Use fake test keys to avoid GitHub push protection false positives

## [Unreleased]

### Planned

- Batch operations optimization
- Streaming job results
- Enhanced metrics integration
- OpenTelemetry tracing support

[1.0.20]: https://github.com/spooled-cloud/spooled-sdk-go/releases/tag/v1.0.20
[1.0.19]: https://github.com/spooled-cloud/spooled-sdk-go/releases/tag/v1.0.19
[1.0.18]: https://github.com/spooled-cloud/spooled-sdk-go/releases/tag/v1.0.18
[1.0.17]: https://github.com/spooled-cloud/spooled-sdk-go/releases/tag/v1.0.17
[1.0.16]: https://github.com/spooled-cloud/spooled-sdk-go/releases/tag/v1.0.16
[1.0.15]: https://github.com/spooled-cloud/spooled-sdk-go/releases/tag/v1.0.15
[1.0.14]: https://github.com/spooled-cloud/spooled-sdk-go/releases/tag/v1.0.14
[1.0.13]: https://github.com/spooled-cloud/spooled-sdk-go/releases/tag/v1.0.13
[1.0.12]: https://github.com/spooled-cloud/spooled-sdk-go/releases/tag/v1.0.12
[1.0.11]: https://github.com/spooled-cloud/spooled-sdk-go/releases/tag/v1.0.11
[1.0.10]: https://github.com/spooled-cloud/spooled-sdk-go/releases/tag/v1.0.10
[1.0.9]: https://github.com/spooled-cloud/spooled-sdk-go/releases/tag/v1.0.9
[1.0.8]: https://github.com/spooled-cloud/spooled-sdk-go/releases/tag/v1.0.8
[1.0.7]: https://github.com/spooled-cloud/spooled-sdk-go/releases/tag/v1.0.7
[1.0.6]: https://github.com/spooled-cloud/spooled-sdk-go/releases/tag/v1.0.6
[1.0.4]: https://github.com/spooled-cloud/spooled-sdk-go/releases/tag/v1.0.4
[1.0.3]: https://github.com/spooled-cloud/spooled-sdk-go/releases/tag/v1.0.3
[1.0.2]: https://github.com/spooled-cloud/spooled-sdk-go/releases/tag/v1.0.2
[1.0.0]: https://github.com/spooled-cloud/spooled-sdk-go/releases/tag/v1.0.0
[Unreleased]: https://github.com/spooled-cloud/spooled-sdk-go/compare/v1.0.20...HEAD
