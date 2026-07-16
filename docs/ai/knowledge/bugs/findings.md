# Findings (Go SDK)

| ID | Sev | Summary | Evidence |
|----|-----|---------|----------|
| GS-01 | P2 | Unset MaxRetries/TimeoutSeconds wire as 0 | `spooled/grpc/client.go` ~141–149 |
| GS-02 | P2 | ~~Worker list/detail shapes drifted from backend REST JSON~~ **FIXED working tree** | `spooled/resources/workers.go` |

Backend ≥0.1.107 maps 0→QUEUE_DEFAULT_* (default 3/300); still diverges if caller wanted “omit means settings default ≠3”.
