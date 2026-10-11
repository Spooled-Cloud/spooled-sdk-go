# Transport

- BaseURL `https://api.spooled.cloud`; timeout 30s; retry 3.
- gRPC enqueue uses bare `int32` for MaxRetries/TimeoutSeconds (`spooled/grpc/client.go` ~141–149) → unset sends 0 → backend maps to 3/300.
- Retry: GET/PUT/DELETE retry on network errors, timeouts, 408, 429 (rate limit) and 5xx; POST only when `Request.Idempotent` (worker acks, via `Base.PostAck`). `RetryPolicy` jitter uses the package-level `math/rand` (safe for concurrent use; a per-policy `*rand.Rand` raced).
- Circuit breaker: shared by every request of a client. A final retryable error counts as one failure; 4xx (including 429 QUOTA_EXCEEDED) and caller-canceled requests do not. `Request.SkipCircuitBreaker` (acks) neither checks nor records.
- `Client.GRPC()` dials lazily with `Config.GRPC`; `Client.Close()` closes that connection.

