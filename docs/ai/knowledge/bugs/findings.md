# Findings (Go SDK)

| ID | Sev | Summary | Evidence |
|----|-----|---------|----------|
| GS-01 | P2 | ~~Unset MaxRetries/TimeoutSeconds always wire as 0~~ **Mitigated (1.1.0)**: `*int32` for API-level nil-vs-0 clarity; wire bytes unchanged (proto3 zeros were never serialized), explicit 0 still inexpressible over gRPC | `spooled/grpc/client.go` |
| GS-02 | P2 | ~~Worker list/detail shapes drifted from backend REST JSON~~ **Fixed (1.1.0)**; also `Deregister` now POSTs `/workers/{id}/deregister` (DELETE always 405'd) | `spooled/resources/workers.go` |
| GS-03 | P1 | ~~`CheckEmail` POSTed `/auth/email/check` (404)~~ **FIXED** | `spooled/resources/auth.go`; backend is `GET /auth/check-email` |
| GS-04 | P1 | ~~`StartEmailLogin` typed `success` the API never sends (always false)~~ **FIXED** | `spooled/resources/auth.go`; body is `message` + `email_sent_to` |
| GS-05 | P1 | ~~Webhook token methods called `/organizations/{id}/webhook-token` (404)~~ **FIXED** | `spooled/resources/organizations.go`; routes are auth-scoped `/organizations/webhook-token` |
| GS-06 | P1 | ~~`Usage` called `/organizations/{id}/usage` (404)~~ **FIXED** | `spooled/resources/organizations.go`; route is auth-scoped `GET /organizations/usage` |
| GS-07 | P1 | ~~`CheckSlug` called `/check-slug/{slug}` and typed `slug`/`message`~~ **FIXED** | `spooled/resources/organizations.go`; route is `GET /check-slug?slug=` → `available`/`valid`/`error`/`suggestion` |
| GS-08 | P1 | ~~`Jobs().List` RetryCount always 0; list JSON sends `attempt`~~ **FIXED** | `spooled/resources/jobs.go` |
| GS-09 | P1 | ~~`BatchStatus` dropped `retry_count`/`queue_name`/`created_at`/`completed_at`~~ **FIXED** | `spooled/resources/jobs.go` |
| GS-10 | P1 | ~~`Workflows().Get()` left `TotalJobs`/`CompletedJobs`/`FailedJobs` at 0~~ **FIXED** | `spooled/resources/workflows.go`; GET detail puts counts under `progress` |
| GS-11 | P2 | ~~`Jobs().List` dropped `job_type` from summaries~~ **FIXED** | `spooled/resources/jobs.go`; `GET /jobs` sends `job_type` from payload |
| GS-12 | P1 | ~~`GetStats` typed flat totals the API never sends~~ **FIXED** | `spooled/resources/admin.go`; `GET /admin/stats` is nested `{organizations,jobs,workers,system}` |
| GS-13 | P1 | ~~`GetPlans` typed nested `limits`/`price` the API never sends~~ **FIXED** | `spooled/resources/admin.go`; `GET /admin/plans` is a flat `PlanLimits` array |
| GS-14 | P1 | ~~`Auth().Validate` dropped `claims` (org/queues/exp always empty)~~ **FIXED** | `spooled/resources/auth.go`; POST `/auth/validate` is `{valid,error,claims}` |
| GS-15 | P1 | ~~`Auth().Logout` omitted refresh token so `/auth/refresh` survived~~ **FIXED** | `spooled/resources/auth.go`; POST `/auth/logout` body `refresh_token` |
| GS-16 | P1 | ~~`Schedules().Create` left `IsActive` false: POST returns `{id,name,cron_expression,next_run_at}` unmarshalled into `Schedule`~~ **FIXED** | `spooled/resources/schedules.go`; create inserts `is_active TRUE` |
| GS-17 | P1 | ~~`Queues().UpdateConfig` PUT `/queues/{name}` (405); config is PUT `/queues/{name}/config`~~ **FIXED** | `spooled/resources/queues.go` |
| GS-18 | P1 | ~~`Metrics().Get` JSON-decoded `/api/v1/metrics`; scrape is `GET /metrics` Prometheus text~~ **FIXED** | `spooled/resources/metrics.go` |
| GS-19 | P1 | ~~`Ingest().CustomWithToken` treated empty 200 as error; `Custom` unmarshalled `{job_id,created}` the API never sends~~ **FIXED** | `spooled/resources/ingest.go`; handler returns `StatusCode::OK` |
| GS-20 | P1 | ~~`Ingest().Custom` returned `error` only and dropped `job_id` after backend started sending `WebhookResponse`~~ **FIXED** | `spooled/resources/ingest.go`; maps `job_id`/`queue_name`/`status` |
| GS-21 | P1 | ~~`Auth().VerifyEmail` dropped `signup_token` (typed login tokens only)~~ **FIXED** | `spooled/resources/auth.go`; POST `/auth/email/verify` is tagged login\|signup |
| GS-22 | P1 | ~~`Job.Payload`/`Result` as `map[string]any` failed to unmarshal non-object JSON~~ **FIXED** | `spooled/resources/jobs.go`; backend payload/result is `serde_json::Value` |
| GS-23 | P1 | ~~`Schedule.PayloadTemplate`/`Tags`/`Metadata` as `map[string]any` failed to unmarshal non-object JSON~~ **FIXED** | `spooled/resources/schedules.go`; backend is `serde_json::Value` |

Backend ≥0.1.107 maps 0→QUEUE_DEFAULT_* (default 3/300); still diverges if caller wanted “omit means settings default ≠3”.
| SDKGP-01 | P1 | ~~`Queues().GetStats` failed for any queue with completed jobs: `avg_processing_time_ms` is a float, SDK typed `*int` (also `types/` and dashboard)~~ **FIXED (1.4.0)** | `spooled/resources/queues.go; spooled/types/{queues,dashboard}.go` |
| SDKGP-02 | P1 | ~~Worker did not recover handler panics; one bad job crashed the process~~ **FIXED (1.4.0)** | `spooled/worker/worker.go callHandler` |
| SDKGP-03 | P1 | ~~`Stop` / cancelling Start's context cancelled in-flight jobs and reported them failed ("context canceled")~~ **FIXED (1.4.0)** | `spooled/worker/worker.go shutdown/abortRemainingJobs` |
| SDKGP-04 | P1 | ~~gRPC `Enqueue`/`Complete` silently dropped a payload/result containing non-`[]any` types~~ **FIXED (1.4.0)** | `spooled/grpc/client.go toStruct` |
| SDKGP-05 | P2 | ~~gRPC TLS auto-detection inverted for localhost; `Client.GRPC()` had no TLS options~~ **FIXED (1.4.0)** | `spooled/grpc/client.go defaultUseTLS; spooled/config.go GRPCOptions` |
| SDKGP-06 | P2 | ~~Realtime clients never reconnected by default~~ **FIXED (1.4.0)** | `spooled/realtime/types.go applyDefaults` |
| SDKGP-07 | P2 | ~~SSE (64 KiB) / WebSocket (32 KiB) read limits dropped the connection on large events~~ **FIXED (1.4.0)** | `spooled/realtime/{sse,websocket}.go` |
| SDKGP-08 | P2 | ~~`JobEvent.Status` never set for status changes (server sends new_status/old_status)~~ **FIXED (1.4.0)** | `spooled/realtime/types.go JobEvent.UnmarshalJSON` |
| SDKGP-09 | P2 | ~~`SpooledWorker.Start` after a failed registration returned nil and never polled~~ **FIXED (1.4.0)** | `spooled/client.go` |
| SDKGP-10 | P2 | ~~`JobContext.Progress` / `Jobs().UpdateProgress` always 404~~ **FIXED (1.4.0)** | `spooled/resources/jobs.go; spooled/worker` (typed ErrUnsupported, no request) |
| SDKGP-11 | P2 | ~~Worker hid non-object payloads from handlers~~ **FIXED (1.4.0)** | `spooled/worker/types.go RawPayload` |
| SDKGP-12 | P2 | ~~Fail text empty / over 2048 characters was rejected by `/fail`~~ **FIXED (1.4.0)** | `spooled/resources/jobs.go CoerceFailureMessage` |
| SDKGP-13 | P2 | ~~`Ingest().Custom` always 401 (no token); `GitHub`/`Stripe` always 404~~ **FIXED (1.4.0)** | `spooled/resources/ingest.go` |
| SDKGP-14 | P2 | ~~Schedule / webhook-delivery list helpers sent filters the API ignores~~ **FIXED (1.4.0)** | `spooled/resources/{schedules,webhooks}.go` (client-side) |
| SDKGP-15 | P2 | ~~`RetryPolicy` shared `*rand.Rand` raced under -race~~ **FIXED (1.4.0)** | `internal/httpx/retry.go` |
| SDKGP-16 | P2 | ~~Payload/result integers above 2^53 corrupted on decode~~ **FIXED (1.4.0)** | `spooled/resources/numbers.go` (json.Number for those) |
| SDKGP-17 | P2 | ~~gRPC `Fail` without `Retry` dead-lettered immediately~~ **FIXED (1.4.0)** | `spooled/grpc/client.go FailRequest.NoRetry` |
| SDKGP-18 | P2 | ~~SSE put the API key in `?api_key=`~~ **FIXED (1.4.0)** | `spooled/realtime/sse.go` |
