# Spooled Go SDK — Agent Knowledge Base

Module tag **v1.6.0**; `internal/version.Version` + User-Agent `spooled-go/1.6.0 (GOOS; GOARCH)`. Requires Go 1.26+ (`go 1.26.0` in go.mod since 1.5.1).

## Surface
REST + worker + realtime + full gRPC. Auth REST Bearer; gRPC `x-api-key`.
Job progress is NOT supported by the API: `Jobs().UpdateProgress` and `JobContext.Progress` return `ErrUnsupported`
without a request, as do `Ingest().GitHub/Stripe` and `Organizations().ClearWebhookToken`.
Worker semantics (drain on stop, panics, job timeouts, ack retries): `contracts/parity.md`, `risk/chesterton-fences.md`.
`NewClient` reads `SPOOLED_API_KEY` / `SPOOLED_BASE_URL` / `SPOOLED_GRPC_ADDRESS` when the option is unset (Unreleased).
Worker ids are global across orgs: never document a fixed id (`contracts/parity.md`).
