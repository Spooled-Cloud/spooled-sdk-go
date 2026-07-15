# Parity notes (Go)

- **Only** SDK with working worker progress → `POST /jobs/{id}/progress`.
- Full gRPC streams.
- Zero-value gRPC fields are a footgun; document for callers.
