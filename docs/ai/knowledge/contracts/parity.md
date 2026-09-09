# Parity notes (Go)

- **Only** SDK with working worker progress → `POST /jobs/{id}/progress`.
- Worker list has a summary-specific `ListSummaries()` API; legacy `List()` maps summaries into `Worker` values for compatibility.
- Full gRPC streams.
- Zero-value gRPC fields are a footgun; document for callers.
- gRPC enqueue: `MaxRetries`/`TimeoutSeconds` are `*int32` (nil = server defaults).
- Workflow job list/get/status are not their own REST routes. `GET /workflows/{id}` carries jobs + dependencies; `Jobs().ListJobs` reads that document. `POST /jobs/{id}/dependencies` takes `depends_on` + `dependency_mode` and returns `dependencies_added` / `dependencies_met`.
- `GET /jobs/{id}/dependencies` is `{ job_id, dependencies, dependents, dependencies_met }` with `{ job_id, queue_name, status }` edges, not a nested `job` object or string id lists.
