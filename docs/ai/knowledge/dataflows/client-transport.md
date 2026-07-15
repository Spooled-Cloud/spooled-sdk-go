# Transport

- BaseURL `https://api.spooled.cloud`; timeout 30s; retry 3.
- gRPC enqueue uses bare `int32` for MaxRetries/TimeoutSeconds (`spooled/grpc/client.go` ~141–149) → unset sends 0 → backend maps to 3/300.
