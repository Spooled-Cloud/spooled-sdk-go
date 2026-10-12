# Fences

- Single version source in `internal/version` (historical multi-literal drift).
- Do not reintroduce independent worker version strings.
- Job contexts hang off the worker's `jobsCtx`, never off the Start/claim context: cancelling that context must start a drain, not kill running handlers. They are canceled only when `ShutdownTimeout` expires (`abortRemainingJobs`).
- `activeJob.settled` is a compare-and-swap claimed by exactly one of the handler's own complete/fail, the job-timeout path or the shutdown-timeout path. `abortRemainingJobs` claims it BEFORE cancelling contexts, or a handler returning "context canceled" would win the race and become the recorded failure. Deregister stays last: the server releases a deregistered worker's processing jobs.
- The handler runs in its own goroutine (`callHandler`) so a panic can be recovered, a timeout can release the slot, and shutdown can stop waiting for a handler that ignores its context. Go cannot kill that goroutine.
- `FailRequest.Retry` (gRPC) is a dead field kept for source compatibility; the wire value is `!NoRetry`, always sent: `true` as the field, `false` as the unknown field 4 = 0 from `grpc.SetNoRetry`. The bundled proto keeps `bool retry = 4` where the backend has `optional bool` (`scripts/generate_grpc.sh` rewrites that one line): generating it as `optional` makes `pb.FailRequest.Retry` a `*bool`, a source break for anyone building `pb.FailRequest` (ProcessJobs users). Do not drop the override or the `SetNoRetry` call in `newFailRequest`: without it a plain `false` never reaches the server and `NoRetry` retries.
- `realtime.ConnectionOptions.AutoReconnect` is ignored (reconnect is the default); `DisableAutoReconnect` is the switch, because a Go bool cannot tell "unset" from "false".
- `resources.restoreNumbers` walks decoded results by reflection; any new `any`-typed field is covered automatically, a new custom `UnmarshalJSON` is not (see `contracts/parity.md`).

