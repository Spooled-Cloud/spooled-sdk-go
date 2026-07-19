# Findings (Go SDK)

| ID | Sev | Summary | Evidence |
|----|-----|---------|----------|
| GS-01 | P2 | ~~Unset MaxRetries/TimeoutSeconds always wire as 0~~ **Mitigated (1.1.0)**: `*int32` for API-level nil-vs-0 clarity; wire bytes unchanged (proto3 zeros were never serialized), explicit 0 still inexpressible over gRPC | `spooled/grpc/client.go` |
| GS-02 | P2 | ~~Worker list/detail shapes drifted from backend REST JSON~~ **Fixed (1.1.0)**; also `Deregister` now POSTs `/workers/{id}/deregister` (DELETE always 405'd) | `spooled/resources/workers.go` |

Backend ≥0.1.107 maps 0→QUEUE_DEFAULT_* (default 3/300); still diverges if caller wanted “omit means settings default ≠3”.
