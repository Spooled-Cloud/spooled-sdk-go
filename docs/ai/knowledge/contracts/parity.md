# Parity notes (Go)

- **Only** SDK with working worker progress → `POST /jobs/{id}/progress`.
- Worker list has a summary-specific `ListSummaries()` API; legacy `List()` maps summaries into `Worker` values for compatibility.
- Full gRPC streams.
- Zero-value gRPC fields are a footgun; document for callers.
