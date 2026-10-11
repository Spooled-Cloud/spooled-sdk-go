# Maintenance

| Change | Update |
|--------|--------|
| Version | `internal/version/version.go`, changelog, git tag |
| gRPC enqueue zeros | `spooled/grpc/client.go`, findings, contracts |
| Worker lifecycle (drain, panic recovery, job timeout, ack settle) | `spooled/worker/worker.go`, `worker_lifecycle_test.go`, `contracts/parity.md` |
| Realtime reconnect / limits / auth | `spooled/realtime/{types,sse,websocket}.go`, `realtime_contract_test.go` |
| Deprecated "unsupported" methods | `spooled/resources/unsupported.go`, `contracts/parity.md` |
| Workflow job routes | `contracts/parity.md` |
