# Parity notes (Go)

- **Only** SDK with working worker progress → `POST /jobs/{id}/progress`.
- Worker list has a summary-specific `ListSummaries()` API; legacy `List()` maps summaries into `Worker` values for compatibility.
- Full gRPC streams.
- Zero-value gRPC fields are a footgun; document for callers.
- gRPC enqueue: `MaxRetries`/`TimeoutSeconds` are `*int32` (nil = server defaults).
- Workflow job list/get/status are not their own REST routes. `GET /workflows/{id}` carries jobs + dependencies; `Jobs().ListJobs` reads that document. `POST /jobs/{id}/dependencies` takes `depends_on` + `dependency_mode` and returns `dependencies_added` / `dependencies_met`.
- `GET /workflows/{id}` is `WorkflowDetailResponse`: job counts are under `progress` (`total`/`completed`/`failed`), not top-level `total_jobs` like list/cancel/retry. `Workflow.UnmarshalJSON` maps those onto `TotalJobs`/`CompletedJobs`/`FailedJobs`.
- `GET /jobs/{id}/dependencies` is `{ job_id, dependencies, dependents, dependencies_met }` with `{ job_id, queue_name, status }` edges, not a nested `job` object or string id lists.
- Email availability is `GET /auth/check-email?email=`, not `POST /auth/email/check`. The body is `available`, `exists`, `signup_enabled`.
- Email login start is `POST /auth/email/start` → `{ message, email_sent_to }`, not `{ success, message }`.
- Org webhook token is `GET/POST /organizations/webhook-token` (auth-scoped), not `/organizations/{id}/webhook-token`. Clear is `POST /organizations/webhook-token/clear` with `confirm: true`, not DELETE.
- Org usage is `GET /organizations/usage` (auth-scoped), not `/organizations/{id}/usage`.
- Slug check is `GET /organizations/check-slug?slug=`, not `/organizations/check-slug/{slug}`. Body is `available`, `valid`, `error`, `suggestion`.
- Job list/DLQ summaries send `attempt` and `max_retries`, not `retry_count`. `Jobs().List` maps `attempt` onto `Job.RetryCount`. Detail `GET /jobs/{id}` still uses `retry_count`.
- `GET /jobs` summaries include `job_type` from `payload.job_type`. `Job.JobType` maps that field; GET detail copies it from `payload` when the top-level field is absent.
- `GET /jobs/status` returns `{ id, status, queue_name, retry_count, created_at, completed_at }` (no `attempt`/`max_retries`). `BatchStatus` maps `retry_count` onto `RetryCount`.
