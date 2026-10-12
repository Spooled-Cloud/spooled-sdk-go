# Spooled Go SDK — Agent Knowledge Base

Module tag **v1.5.0**; `internal/version.Version` + User-Agent `spooled-go/1.5.0 (GOOS; GOARCH)`.

## Surface
REST + worker + realtime + full gRPC. Auth REST Bearer; gRPC `x-api-key`.
Job progress is NOT supported by the API: `Jobs().UpdateProgress` and `JobContext.Progress` return `ErrUnsupported`
without a request, as do `Ingest().GitHub/Stripe` and `Organizations().ClearWebhookToken`.
Worker semantics (drain on stop, panics, job timeouts, ack retries): `contracts/parity.md`, `risk/chesterton-fences.md`.
