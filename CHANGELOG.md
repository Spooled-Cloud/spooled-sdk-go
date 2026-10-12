# Changelog

All notable changes to the Spooled Go SDK will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [1.5.0] - 2026-10-12

Catches the SDK up with backend 0.1.122 and fixes what the production check of
1.4.0 found. Everything is additive except the regenerated `spooled/grpc/pb`
package (see **Notes**).

### Fixed

- **gRPC `FailRequest.NoRetry` now dead-letters the job.** The backend made
  `FailRequest.retry` an `optional bool` (absent = retry per `max_retries`), but
  the SDK still generated a plain proto3 `bool`, whose `false` is never put on
  the wire: the server read every `NoRetry: true` as "retry". The bundled proto
  is regenerated from the backend and `Fail` always sends `retry` explicitly
  (`true` by default, `false` for `NoRetry`). The contract test now checks
  presence on the wire instead of `GetRetry()`, which could not tell the two
  apart.
- `lease_id` is required on REST complete, fail and heartbeat (`422
  VALIDATION_ERROR` without it, since backend 0.1.121; it was a `409` before)
  and on gRPC `Complete`, `Fail` and `RenewLease` (`InvalidArgument`, since
  backend 0.1.100). The doc comments said "when set" and "nil = legacy
  worker_id fence"; they now say it is required. The field types are unchanged.
  `examples/grpc` passes `job.LeaseID` (it failed every completion yet printed
  "complete!") and exits non-zero when a settlement fails.
- `Workflows().Jobs().ListJobs`, `GetJob` and `GetJobsStatus` returned an empty
  `Key` for every job: `GET /workflows/{id}` carries no job key. `Key` is now
  `omitempty` and documented as empty, and `CreateWorkflowResponse` gains
  `KeyByJobID()`, `JobID(key)` and `ApplyKeys(jobs)` to map keys from the create
  response (the same approach as the Node and Python SDKs). `examples/workflow`
  prints the keys again.
- A graceful worker shutdown no longer logs `Failed to send worker heartbeat:
  context canceled`: a heartbeat in flight when the drain ends is cancelled on
  purpose.
- `scripts/test-local` matches the current API: it echoes each claim's lease on
  every settlement, accepts `sp_live_` / `sp_test_` keys and a token-protected
  `/metrics` (401), checks that gRPC `NoRetry` dead-letters, and skips the steps
  that create an organization or a billing portal session against production
  unless `RUN_UNSAFE_TESTS=1`. `scripts/test-production.go` finds the module root
  on Windows too.

### Added

- `ListJobsParams`: `Statuses` (sent as one comma-separated `status`), `Q`,
  `JobType`, `CreatedAfter`, `CreatedBefore` (sent as RFC 3339), `OrderBy` and
  `OrderDir`. `ListDLQParams`: `Tag`, `Q`, `JobType`, `CreatedAfter`,
  `CreatedBefore`. The API answers an invalid filter with `400`.
- `Jobs().ListPage` and `Jobs().DLQ().ListPage` return a `JobsPage` with
  `HasMore` from the `X-Has-More` header. `List` is unchanged.
- `Jobs().History(ctx, id, *JobHistoryParams)`: a job's recorded lifecycle
  events (`GET /jobs/{id}/history`, `Order` asc/desc, `Limit`, `Offset`), with
  `HasMore`.
- `Worker` and `WorkerSummary` (and `spooled/types`): `JobsProcessed` and
  `JobsFailed`.
- Outgoing webhooks: `WebhookFormat` (`json`, `slack`, `discord`, `teams`) as
  `Format` on the webhook and on create/update requests, and the events
  `WebhookEventJobDeadlettered`, `WebhookEventJobRetried`,
  `WebhookEventWorkflowCompleted` and `WebhookEventWorkflowFailed`.
- `Ingest().ListDeliveries`: the inbound webhook log (`GET /webhooks/deliveries`,
  filters `Status`, `Since`, `QueueName`, `Limit`, `Offset`), as
  `InboundWebhookDelivery` rows.
- `RateLimitError.Scope` from `X-RateLimit-Scope` (`organization` or `api_key`).
- gRPC: `Client.FailWithResponse` returns the server's `WillRetry` and
  `NextRetryDelaySecs`; `RegisterWorkerRequest.WorkerID` sends a stable worker
  id.
- `spooled/types`: `LeaseID` on `ClaimedJob`, `CompleteJobRequest`,
  `FailJobRequest` and `HeartbeatJobRequest`; the new list filters,
  `JobHistoryEvent`, `InboundWebhookDelivery`, webhook `Format` and events.

### Documentation

- README: the rate-limit section said the API sends only `Retry-After`; it sends
  `X-RateLimit-Limit`, `-Remaining`, `-Reset` (Unix seconds) and `-Scope` too,
  and `RateLimitError` carries them. The README used `WithRetryConfig`,
  `IsRetryableError`, `IsNetworkError`, `IsTimeoutError`, `APIKeys().Revoke`,
  `Queues().Purge`, `QueueStats.Pending` and `[]resources.WorkflowJob` in a
  workflow definition, none of which compile; every Go snippet now builds
  against the SDK. New sections cover the required `LeaseID`, list filters and
  history, workflow keys, webhook formats, the inbound log and the gRPC payload
  limits.
- gRPC payloads and results are `google.protobuf.Struct`: always a JSON object,
  numbers are doubles (integers of 2^53 or more arrive rounded), and a job
  created over REST with a non-object payload arrives over gRPC with an empty
  one. Use REST for those payloads.
- `FailJobResponse.WillRetry` / `NextRetryDelaySec` are documented as nil
  against Spooled Cloud: `POST /jobs/{id}/fail` answers `{"success":true}` only.

### Notes

- `spooled/grpc/pb` is regenerated from backend proto
  `proto/spooled.proto` (commit f84174e, sha256
  f2b5844b505e0f9c76a58ec281d98c4f062bf0f1f7c0cfdb5c834e6c64977bc3) with
  protoc 36.2 (libprotoc), protoc-gen-go v1.36.11 and protoc-gen-go-grpc v1.6.0.
  `pb.FailRequest.Retry` is now `*bool` and `pb.RegisterWorkerRequest` gains
  `WorkerId *string`. Code that builds `pb.FailRequest` itself (for example on a
  `ProcessJobs` stream) must set `Retry: proto.Bool(false)` to dead-letter; the
  high-level `grpc.Client` API is unchanged.
- The test fixture API key is now `sp_test_000000000000000000000000000000`, so
  secret scanners no longer flag the test files.

## [1.4.0] - 2026-10-10

A reliability pass on the worker, the transport, gRPC and the realtime clients,
from a deep bug hunt against the backend contract. Read **Behaviour changes**
before upgrading a worker: shutdown now waits for running handlers.

### Behaviour changes

- **`Stop` drains instead of cancelling.** `Worker.Stop()` and cancelling the
  context passed to `Start` used to cancel every in-flight job context first, so
  context-aware handlers returned `context canceled` and the job was reported
  failed (a job on its last retry went straight to the dead-letter queue during a
  routine deploy). Now they stop claiming, let running handlers finish for up to
  `ShutdownTimeout` with their contexts live, send `complete` / `fail` for every
  handler that returns in that window, and deregister last. Only handlers still
  running at the timeout are cancelled, and their jobs are failed with
  `Worker shutdown timeout`. Cancelling `Start`'s context begins the same drain by
  itself; `Stop` blocks until it is done. `SpooledWorkerOptions` gains
  `ShutdownTimeout`.
- **Handler panics fail the job instead of crashing the process.** The panic is
  recovered, the job is failed with `panic: <value>`, and the worker keeps
  running (SDKGP-02).
- **Job timeouts.** A claimed job with `timeout_seconds > 0` is cancelled and
  failed with `Job timeout exceeded` when its handler runs longer
  (`EventJobTimeout`, `JobTimeoutData`); its lease is no longer renewed and the
  concurrency slot is released.
- **Acknowledgements are retried.** `complete`, `fail`, job heartbeats and worker
  heartbeats retry on network errors, timeouts, `408`, `429` and `5xx` with the
  client's retry settings, and they are sent even while the circuit breaker is
  open. A repeat that meets the server's `409 Job is already ...` after a lost
  response counts as delivered. `408` is now retryable for every request.
- **A plan quota is not transient.** A `429 QUOTA_EXCEEDED` (or a `429` without
  `Retry-After` carrying a plan-limit body) is never retried and no longer
  counts against the circuit breaker, so a full plan cannot block acks.
  `RateLimitError` gains `Quota *QuotaInfo` and `IsQuotaExceeded()`; it still
  matches `IsRateLimitError`. `spooled.IsQuotaExceeded` is new.
- **Failure text is always accepted.** `Jobs().Fail` / `FailWithResponse` and the
  gRPC `Fail` send `Job failed` for an empty message and truncate anything over
  2048 characters on a character boundary; the API rejected both with 400, which
  left the job `processing` until its lease expired. `resources.CoerceFailureMessage`
  is exported.
- **Realtime clients reconnect by default.** A `ConnectionOptions` built without
  `AutoReconnect: true` used to stay disconnected for good after the first drop,
  and the server ends every stream after 30 minutes. Reconnecting is now on unless
  `DisableAutoReconnect: true`; `AutoReconnect` is deprecated and ignored. The
  reconnect counter now resets only after a connection stayed up for 10 s, so
  `MaxReconnectAttempts` can trip.
- **gRPC `Fail` retries by default.** proto3 turned an omitted `Retry` into
  `false`, which dead-letters on the first failure. `FailRequest.NoRetry` is the
  new opt-out; `Retry` is deprecated and ignored.
- **gRPC TLS default.** TLS is used for port 443 (or no port) and plaintext for
  every other port. `localhost:50051` was dialed with TLS because of an inverted
  guard. Use `grpc.ClientOptions.UseTLS`, or `spooled.WithGRPCTLS` /
  `WithGRPCOptions` (`GRPCOptions{UseTLS, TLSConfig, DialOptions, Timeout}`,
  also `Config.GRPC`) with `Client.GRPC()`, for a TLS endpoint on another port.
- **Large integers survive.** Job payload/result/tag numbers decode as `float64`
  exactly as before, except an integer above 2^53 (a Discord ID, a bigint key),
  which is now a `json.Number` instead of a silently rounded `float64`. Code that
  type-asserts such a value as `float64` must accept `json.Number` (`n.Int64()`).

### Fixed

- `Queues().GetStats` failed for any queue that had completed jobs:
  `avg_processing_time_ms` is fractional and was typed `*int`. It, and the
  dashboard `avg_wait_time_ms` / `avg_processing_time_ms` in `spooled/types`, are
  now `*float64` (SDKGP-01).
- gRPC `Enqueue` / `Complete` created the job with `{}` (and lost results) when a
  payload held a `[]string`, `map[string]string`, `time.Time` or struct, and
  reported success. Values are JSON-encoded first, and a value that cannot be
  encoded is returned as an error (SDKGP-04).
- `JobEvent.Status` is filled from `new_status` on `job.status_changed`
  (`OldStatus`, `NewStatus`, `DurationMs`, `Timestamp` added); a non-object
  `result` no longer drops the whole event (`RawResult` has it, `Result` stays the
  object view) (SDKGP-08).
- SSE read the stream with a 64 KiB line limit and WebSocket with a 32 KiB message
  limit, so one large `job.completed` result tore the connection down. Both now
  read up to `MaxMessageBytes` (default 4 MiB) (SDKGP-07).
- The SSE client sends the API key as `Authorization: Bearer`, not in `?api_key=`
  where it reached proxy and CDN logs (SDKGP-18).
- `SpooledWorker.Start` after a failed registration returned `nil` and nothing
  polled; it now returns the error and can be called again. The low-level
  `Worker.Start` is retryable too (SDKGP-09).
- The worker passes non-object payloads to handlers: `JobContext.RawPayload`
  holds any JSON value, and `SpooledWorker` hands it to `Job.Payload` (SDKGP-11).
- `Schedules().List` and `Webhooks().Deliveries` sent `queue_name`, `is_active`,
  `status` and `offset`, which the API ignores, so an offset loop re-read the
  first page forever. They send only what the API honors and apply the filters
  and `Offset` / `Limit` locally (SDKGP-14).
- `RetryPolicy` used one `*rand.Rand` from many goroutines, a data race under
  `-race` in the default configuration (SDKGP-15).
- A cancelled non-idempotent request (a claim poll cancelled at shutdown) no
  longer counts as a circuit-breaker failure.
- `Client.Close` closes the lazily dialed gRPC connection.
- The worker no longer panics on `LeaseDuration * HeartbeatFraction` under one
  second (a zero-length ticker), caps its claim `limit` at the API maximum of 100
  and falls back to `unknown` for an empty hostname.
- SSE: an `event:` line without data no longer leaks its type onto the next event.
- `Health().Liveness()` / `Readiness()` return a `Status` (`alive` / `ready`) for
  the empty 200 the API sends.

### Deprecated

These methods call routes the API does not have. They now fail immediately,
without a network call, with a typed `*resources.UnsupportedError` that matches
`errors.Is(err, spooled.ErrUnsupported)` (`spooled.IsUnsupportedError`), instead
of a 404 that looks like an outage (SDKGP-10, SDKGP-13):

- `Jobs().UpdateProgress` and `JobContext.Progress`: the API has no job progress
  route. A handler that returned the error from `Progress` was failing every job
  with a 404 already; return progress in the result or use `JobContext.Log`.
- `Ingest().GitHub`, `Ingest().Stripe`: the API serves only
  `POST /webhooks/{org_id}/custom`.
- `Organizations().ClearWebhookToken`: the API always answers 422. Rotate with
  `RegenerateWebhookToken`.
- `UpdateScheduleRequest.QueueName` is ignored by the API (documented).

`Ingest().Custom` always got 401 because the API requires the organization's
webhook token. Set `CustomWebhookRequest.WebhookToken` (or call
`CustomWithToken`); without a token `Custom` returns
`resources.ErrWebhookTokenRequired` before any request.

### Migration

- A reconnecting realtime client that you turned off with `AutoReconnect: false`
  must now set `DisableAutoReconnect: true`.
- gRPC code that relied on `FailRequest{}` dead-lettering must set
  `NoRetry: true`; `Retry: true` can simply be removed.
- A gRPC server on a non-443 port that terminates TLS needs `UseTLS` / `WithGRPCTLS(true)`.
- Do not call `Worker.Stop` from inside a handler or event callback; it waits for
  handlers.

## [1.3.1] - 2026-09-26

### Fixed

- **`Jobs().DLQ().Purge` always failed.** The API requires `"confirm": true`
  on `POST /jobs/dlq/purge`, and `PurgeDLQRequest` had no such field, so every
  purge was rejected as a validation error. `Purge` now always sends it (calling
  `Purge` is the confirmation) without mutating the caller's request, and a nil
  request purges the whole DLQ. `PurgeDLQRequest` also gains the `OlderThan` and
  `Limit` filters the API accepts.

### Notes

- `Jobs().Heartbeat` with `LeaseDurationSec` left nil was a 422 on API versions
  before 0.1.115; from 0.1.115 the server applies the default lease (30 s).
- The SSE client already sends `?queue=` / `?job_id=` to `/api/v1/events`; with
  API 0.1.114+ that stream carries job events, so `OnJobEvent` handlers fire.

## [1.3.0] - 2026-09-10

A contract-parity pass against the backend: every fix below is a place where
this SDK's route, request shape, or response mapping disagreed with what the API
actually serves. Pair it with backend `0.1.112`, which supplies the response
fields several of these now read.

### Breaking

- **JSON-valued fields are now `any` instead of `map[string]any`.** The API
  types job payloads, results, tags, schedule payload templates, workflow job
  payloads and worker/workflow metadata as `serde_json::Value`, so a JSON
  array, string, number or boolean is valid in all of them. Modelling these as
  a Go map meant `encoding/json` refused to decode such a value at all —
  `GET /jobs/{id}` on a job with an array payload returned an error rather than
  the job — and the SDK could not send one either. Affected exported fields:
  `Job.Payload` / `.Result` / `.Tags`, `ClaimedJob.Payload`,
  `CreateJobRequest.Payload` / `.Tags`, `BulkJobItem.Payload`,
  `CompleteJobRequest.Result`, `WorkflowJobDefinition.Payload`, and the
  schedule payload/tags/metadata fields, in both `spooled/types` and
  `spooled/resources`. Code that constructs these with a map literal still
  compiles; code that indexes them directly needs a type assertion first
  (`payload, ok := job.Payload.(map[string]any)`).
- **`resources.CustomWebhookResponse.Created` was removed**, replaced by
  `QueueName` and `Status`. The field was never populated: the endpoint
  answered `200` with an empty body, so `Created` was always `false`.

### Fixed

- `Ingest().Custom` / `CustomWithToken` now map OpenAPI `WebhookResponse`
  (`job_id`, `queue_name`, `status`). They previously required `{job_id,
  created}` (an empty 200 looked like a failure), then returned `error` only and
  dropped the job id the API actually sends. An empty 200 — a backend older than
  `0.1.112` — still maps to a zero response rather than an error.
- `Auth().VerifyEmail` now maps the tagged `{type: login|signup}` body,
  including `signup_token` for new emails. It previously unmarshalled only
  token fields, so a signup response left `AccessToken` empty and dropped
  the token needed for `POST /auth/signup/complete`.
- `Auth().Logout` now sends the refresh token in the body. Without it the
  access token is blacklisted but `/auth/refresh` still mints a new pair, so
  logout did not end the session. The stored refresh token is used when the
  argument is omitted, matching the Node, PHP, and Python SDKs.
- `Auth().Validate` now maps `claims.org_id` / `api_key_id` / `queues` / `exp`
  onto `OrganizationID` / `APIKeyID` / `Queues` / `ExpiresAt`. It previously
  looked for top-level `organization_id` and `expires_at`, which the API never
  sends, so a valid token unmarshalled as empty IDs.
- `Auth().CheckEmail` now calls `GET /auth/check-email?email=`. It previously
  POSTed to `/auth/email/check`, which is not a backend route, so every check
  404'd. The response now also reads `available` and `signup_enabled`.
- `Auth().StartEmailLogin` now reads `message` and `email_sent_to` from
  `POST /auth/email/start`. It previously typed a `success` field the API never
  sends, so a successful send unmarshalled as `Success: false`.
- `Organizations().GetWebhookToken` / `RegenerateWebhookToken` now call
  `GET/POST /organizations/webhook-token`. They previously used
  `/organizations/{id}/webhook-token`, which is not a backend route, so every
  call 404'd. `ClearWebhookToken` now POSTs `/organizations/webhook-token/clear`
  with `confirm: true` instead of DELETE on the missing id path.
- `Organizations().Usage` now calls `GET /organizations/usage`. It previously
  used `/organizations/{id}/usage`, which is not a backend route, so every
  call 404'd.
- `Organizations().CheckSlug` now calls `GET /organizations/check-slug?slug=`.
  It previously requested `/organizations/check-slug/{slug}`, which is not a
  backend route, so every check 404'd. The response now reads `valid`,
  `error`, and `suggestion` instead of `slug`/`message`, which the API never
  sends.
- `Jobs().List` / `Jobs().DLQ().List` now map `attempt` onto `RetryCount`.
  List/DLQ summaries send `attempt`, not `retry_count`, so every listed job
  looked like it had never been retried. They also read the `job_type` and
  `last_error` that backend `0.1.112` adds to `JobSummary`.
- `Jobs().BatchStatus` now reads `queue_name`, `retry_count`, `created_at`,
  and `completed_at`. `GET /jobs/status` always sent those fields; the SDK
  previously decoded only `id` and `status`.
- `Queues().UpdateConfig` now sends `PUT /queues/{name}/config`. It previously
  PUT to `/queues/{name}`, which is not a backend route.
- `Workflows().Get` now reads the workflow's jobs from `GET /workflows/{id}`,
  and `Workflows().Jobs().GetDependencies` unmarshals
  `GET /jobs/{id}/dependencies` in the shape the API sends. Workflow progress
  counts are read from `progress`, which is where `total`/`completed`/`failed`
  live; there is no top-level `total_jobs`.
- `Schedules().Create` maps its response onto an active schedule instead of
  leaving `IsActive` false on a schedule the server had already started.
- `Admin().UpdateOrganization` now sends `PATCH /admin/organizations/{id}` (the
  route is PATCH, not PUT) and `Admin().DeleteOrganization` sends the
  `hard_delete` query parameter.
- `Admin().GetStats` now reads nested `{organizations, jobs, workers, system}`
  from `GET /admin/stats`. It previously typed flat `total_organizations` /
  `total_jobs` fields the API never sends, so every count unmarshalled as 0.
- `Admin().GetPlans` now decodes the flat `PlanLimits` array from
  `GET /admin/plans`. It previously typed a nested `{limits, price, description}`
  wrapper the API never sends, so every limit unmarshalled as 0.
- `Metrics()` scrapes `GET /metrics` as Prometheus text rather than trying to
  decode it as JSON.

### Changed

- Regenerated `internal/openapi` types from the current backend OpenAPI spec.

## [1.2.0] - 2026-08-16

### Added

- Optional stable `worker_id` on worker registration. Supplying the same id
  across restarts makes registration an upsert, so a restarting worker reuses
  its row instead of leaving the old one against the plan worker cap until the
  stale-worker reaper clears it (~2 minutes). Omitting it keeps the previous
  behaviour of a server-minted UUID. An id owned by another organization
  returns 409.
- `auto_disabled` in the outgoing-webhook `last_status` domain. The backend now
  disables a webhook after 20 consecutive failed deliveries; re-enable it by
  setting `enabled: true`, which is charged against the plan webhook cap and can
  therefore return `429 QUOTA_EXCEEDED`.

### Changed

- Webhook updates can now express clearing the signing secret distinctly from
  leaving it alone. Backend 0.1.111 treats an explicit `null` as a destructive
  clear, so an untouched secret must be omitted rather than serialised.
- `failure_count` on outgoing webhooks is now counted once per delivery rather
  than once per retry attempt, so for the same failures it is roughly 5x smaller
  than before.
- Documented that `last_used` on API keys is coarse (written at most once per
  key per five minutes) and that webhook delivery history is retained by plan
  rather than kept indefinitely.

### Note

- Backend 0.1.111 no longer accepts API keys in the query string on REST
  endpoints. This SDK sends credentials as an `Authorization` header on REST;
  SSE and WebSocket connections continue to use the query string, which the
  backend still supports for those routes.

## [1.1.0] - 2026-07-19

### Added

- Added `Workers().ListSummaries()` for the worker list endpoint's summary response shape; legacy `List()` remains for compatibility.
- Added `grpc.Int32()` helper for building the new pointer fields on `EnqueueRequest`.

### Changed

- **BREAKING (Go API):** gRPC `EnqueueRequest.MaxRetries` / `TimeoutSeconds` are now `*int32` so `nil` explicitly means "use server `QUEUE_DEFAULT_*`". This is a source-level clarity change only — plain proto3 `int32` zero values were never serialized, so wire bytes are unchanged, and an explicit `0` remains inexpressible over gRPC. Migration: replace `MaxRetries: 3` with `MaxRetries: grpc.Int32(3)`.

### Fixed

- Worker detail decoding now covers `queue_names` and `updated_at`, matching the backend's stable public worker detail response.
- `Workers().Deregister()` now calls `POST /api/v1/workers/{id}/deregister` (the route the backend serves) instead of `DELETE /api/v1/workers/{id}`, which always returned 405 and left the worker registered.

## [1.0.21] - 2026-07-12

### Fixed

- Removed two stale macOS executables that were tracked at the repository root and therefore distributed inside Go module archives.
- Added ignore rules preventing those local build outputs from being recommitted.
- Added release identity validation for strict SemVer tags, the embedded SDK version, and dated changelog entries.
- Expanded maintainer release guidance with runtime/User-Agent, both worker paths, proxy, checksum, and module-archive verification.
- Clarified the `v1.0.10` retraction as tag-mutation provenance ambiguity rather than asserting a currently observable proxy-content mismatch.

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

- v1.0.10 — release activity occurred on more than one tag target, making
  provenance ambiguous after the externally visible tag was moved; use v1.0.11+

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

[1.1.0]: https://github.com/spooled-cloud/spooled-sdk-go/releases/tag/v1.1.0
[1.0.21]: https://github.com/spooled-cloud/spooled-sdk-go/releases/tag/v1.0.21
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
[Unreleased]: https://github.com/spooled-cloud/spooled-sdk-go/compare/v1.1.0...HEAD
