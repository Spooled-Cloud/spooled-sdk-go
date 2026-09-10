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

Backend ≥0.1.107 maps 0→QUEUE_DEFAULT_* (default 3/300); still diverges if caller wanted “omit means settings default ≠3”.
