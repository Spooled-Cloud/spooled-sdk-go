package worker

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime/debug"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/spooled-cloud/spooled-sdk-go/internal/httpx"
	"github.com/spooled-cloud/spooled-sdk-go/spooled/resources"
)

// Failure texts the SDK reports itself. They are a string type, not
// errors.New values, so the capitalised text the cross-SDK contract fixes
// ("Job timeout exceeded", "Worker shutdown timeout") does not trip the
// error-string style check.
type sdkFailure string

func (e sdkFailure) Error() string { return string(e) }

const (
	errJobTimeout      sdkFailure = "Job timeout exceeded"
	errShutdownTimeout sdkFailure = "Worker shutdown timeout"
)

const (
	// ackTimeout bounds one complete/fail acknowledgement including the
	// transport's retries on network errors, timeouts and 5xx. It must outlast
	// the retry backoff, or the retries the transport is configured for never run.
	ackTimeout = 60 * time.Second
	// renewTimeout bounds one lease renewal or worker heartbeat including retries.
	renewTimeout = 15 * time.Second
	// maxClaimBatch is the largest limit POST /jobs/claim accepts.
	maxClaimBatch = 100
)

// activeJob tracks an in-progress job.
type activeJob struct {
	jobID          string
	queueName      string
	workerID       string
	retryCount     int
	maxRetries     int
	timeoutSeconds int
	ctx            context.Context
	cancel         context.CancelFunc
	startTime      time.Time
	heartbeat      *time.Ticker
	leaseLost      atomic.Bool
	// settled is claimed by exactly one of: the handler's own complete/fail, the
	// job-timeout path, or the shutdown-timeout path. Whoever claims it sends the
	// acknowledgement; everyone else stays silent so a job is never settled twice.
	settled atomic.Bool
	// settleDone is closed when the claimant's acknowledgement has finished, so
	// shutdown can wait for an ack that is already in flight before it
	// deregisters. Nil in unit tests that build an activeJob by hand.
	settleDone chan struct{}
	// abandon is closed when shutdown gives up on this job. The job goroutine then
	// stops waiting for a handler that ignores cancellation.
	abandon     chan struct{}
	abandonOnce sync.Once
	// leaseID is the immutable lease fencing token captured at claim time; it
	// must never be looked up again by job ID because a later execution of the
	// same job may already hold a different lease.
	leaseID *string
}

// beginSettle claims the right to acknowledge this job.
func (aj *activeJob) beginSettle() bool { return aj.settled.CompareAndSwap(false, true) }

// endSettle marks the claimed acknowledgement as finished.
func (aj *activeJob) endSettle() {
	if aj.settleDone != nil {
		close(aj.settleDone)
	}
}

func (aj *activeJob) abandonJob() {
	if aj.abandon != nil {
		aj.abandonOnce.Do(func() { close(aj.abandon) })
	}
}

// Worker processes jobs from a Spooled queue using REST polling.
type Worker struct {
	jobs    *resources.JobsResource
	workers *resources.WorkersResource
	opts    Options

	state    atomic.Value // State
	workerID string
	handler  JobHandler

	activeJobs sync.Map // map[string]*activeJob
	jobCount   atomic.Int32

	pollTicker      *time.Ticker
	heartbeatTicker *time.Ticker
	eventHandlers   []EventHandler

	mu sync.RWMutex
	// ctx drives claiming. It is canceled the moment shutdown begins (Stop, or
	// the context given to Start ending), which stops new jobs from being
	// claimed. It deliberately does NOT parent the job contexts: a running
	// handler must be allowed to finish and be acknowledged.
	ctx    context.Context
	cancel context.CancelFunc
	// jobsCtx parents every job context. It is canceled only when the shutdown
	// timeout expires and the remaining handlers are aborted.
	jobsCtx    context.Context
	jobsCancel context.CancelFunc
	// hbCtx drives the worker heartbeat, which keeps running while jobs drain so
	// the scheduler does not mark a busy, shutting-down worker offline.
	hbCtx    context.Context
	hbCancel context.CancelFunc

	pollWG   sync.WaitGroup // the poll loop
	hbWG     sync.WaitGroup // the worker heartbeat loop
	jobWG    sync.WaitGroup // running job goroutines
	stopDone chan struct{}  // closed when a shutdown in progress has finished
}

// NewWorker creates a new REST polling worker.
func NewWorker(jobs *resources.JobsResource, workers *resources.WorkersResource, opts Options) *Worker {
	defaults := DefaultOptions()

	if opts.Concurrency == 0 {
		opts.Concurrency = defaults.Concurrency
	}
	if opts.PollInterval == 0 {
		opts.PollInterval = defaults.PollInterval
	}
	if opts.LeaseDuration == 0 {
		opts.LeaseDuration = defaults.LeaseDuration
	}
	if opts.HeartbeatFraction == 0 {
		opts.HeartbeatFraction = defaults.HeartbeatFraction
	}
	if opts.ShutdownTimeout == 0 {
		opts.ShutdownTimeout = defaults.ShutdownTimeout
	}
	if opts.WorkerType == "" {
		opts.WorkerType = defaults.WorkerType
	}
	if opts.Version == "" {
		opts.Version = defaults.Version
	}
	if opts.Hostname == "" {
		// An empty hostname is rejected by registration (422).
		if hostname, err := os.Hostname(); err == nil && hostname != "" {
			opts.Hostname = hostname
		} else {
			opts.Hostname = "unknown"
		}
	}

	w := &Worker{
		jobs:    jobs,
		workers: workers,
		opts:    opts,
	}
	w.state.Store(StateIdle)

	return w
}

// Process registers a job handler.
func (w *Worker) Process(handler JobHandler) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.handler = handler
}

// Start registers the worker and starts claiming jobs. It returns once the
// worker is running.
//
// Cancelling ctx shuts the worker down exactly like Stop: it stops claiming,
// lets running handlers finish for up to Options.ShutdownTimeout, acknowledges
// them, and deregisters. Handlers are not canceled while they drain, so wait for
// that to finish with Stop (which blocks until it has):
//
//	cancel() // e.g. from a SIGTERM handler
//	w.Stop() // returns after the drain
//
// If registration fails, Start returns the error and may be called again.
func (w *Worker) Start(ctx context.Context) error {
	w.mu.Lock()
	if w.handler == nil {
		w.mu.Unlock()
		return fmt.Errorf("no job handler registered; call Process() first")
	}

	if st := w.state.Load().(State); st != StateIdle && st != StateError {
		w.mu.Unlock()
		return fmt.Errorf("worker already started")
	}

	w.state.Store(StateStarting)
	w.ctx, w.cancel = context.WithCancel(ctx)
	// Job and heartbeat contexts keep the caller's values but not its
	// cancellation: ctx ending must start a drain, not kill running handlers.
	detached := context.WithoutCancel(ctx)
	w.jobsCtx, w.jobsCancel = context.WithCancel(detached)
	w.hbCtx, w.hbCancel = context.WithCancel(detached)
	w.mu.Unlock()

	// Register worker with API
	concurrency := w.opts.Concurrency
	version := w.opts.Version
	workerType := w.opts.WorkerType
	metadata := make(map[string]any)
	for k, v := range w.opts.Metadata {
		metadata[k] = v
	}

	// Only send worker_id when the caller pinned one. Sending it makes
	// registration an upsert, so a restarting worker reuses its existing row
	// instead of leaving a stale one against the plan worker cap.
	var workerIDPtr *string
	if w.opts.WorkerID != "" {
		workerIDPtr = &w.opts.WorkerID
	}

	resp, err := w.workers.Register(ctx, &resources.RegisterWorkerRequest{
		WorkerID:       workerIDPtr,
		QueueName:      w.opts.QueueName,
		Hostname:       w.opts.Hostname,
		MaxConcurrency: &concurrency,
		Version:        &version,
		WorkerType:     &workerType,
		Metadata:       metadata,
	})
	if err != nil {
		w.cancel()
		w.jobsCancel()
		w.hbCancel()
		// StateError (not Idle) so State() reports the failure, but Start accepts it
		// again: a transient 503/429 on register must be retryable.
		w.state.Store(StateError)
		return fmt.Errorf("failed to register worker: %w", err)
	}

	// The tickers exist before the worker is marked running, so a Stop racing
	// with Start never sees a half-built worker.
	w.pollTicker = time.NewTicker(w.opts.PollInterval)
	w.heartbeatTicker = time.NewTicker(w.heartbeatInterval())

	w.mu.Lock()
	w.workerID = resp.ID
	w.state.Store(StateRunning)
	w.mu.Unlock()

	w.emit(Event{
		Type:      EventWorkerStarted,
		Timestamp: time.Now(),
		Data:      WorkerStartedData{WorkerID: w.workerID, QueueName: w.opts.QueueName},
	})

	// Start polling
	w.pollWG.Add(1)
	go w.pollLoop()

	// Start worker heartbeat
	w.hbWG.Add(1)
	go w.workerHeartbeatLoop()

	// ctx ending is a shutdown request.
	go w.watchStartContext(ctx)

	w.log("Worker started: id=%s queue=%s", w.workerID, w.opts.QueueName)
	return nil
}

// watchStartContext turns the end of the context given to Start into a graceful
// Stop. It exits quietly when Stop got there first.
func (w *Worker) watchStartContext(parent context.Context) {
	<-w.ctx.Done()
	if parent.Err() != nil {
		_ = w.Stop()
	}
}

// Stop gracefully stops the worker and blocks until it has finished.
//
// It stops claiming new jobs, then waits up to Options.ShutdownTimeout for
// running handlers to finish. A handler that returns in that window has its
// result sent (complete, or fail if it returned an error) before the worker
// deregisters, whatever the reason Stop was called. Only handlers still running
// when the timeout expires are canceled (their JobContext.Context is done) and
// their jobs failed with "Worker shutdown timeout". The worker is deregistered
// last. Stop is safe to call more than once and from several goroutines.
func (w *Worker) Stop() error {
	w.mu.Lock()
	switch w.state.Load().(State) {
	case StateRunning:
		w.state.Store(StateStopping)
		done := make(chan struct{})
		w.stopDone = done
		workerID := w.workerID
		w.mu.Unlock()

		w.shutdown(workerID)
		close(done)
		return nil
	case StateStopping:
		done := w.stopDone
		w.mu.Unlock()
		if done != nil {
			<-done
		}
		return nil
	default:
		w.mu.Unlock()
		return nil
	}
}

func (w *Worker) shutdown(workerID string) {
	w.log("Stopping worker: id=%s", workerID)

	// 1. Stop claiming. Cancelling ctx ends the poll loop and any claim in flight;
	// waiting for the loop guarantees no further job can start after this point.
	w.cancel()
	if w.pollTicker != nil {
		w.pollTicker.Stop()
	}
	w.pollWG.Wait()

	// 2. Let running handlers finish, with their contexts still live.
	drained := make(chan struct{})
	go func() {
		w.jobWG.Wait()
		close(drained)
	}()
	reason := "graceful shutdown"
	timer := time.NewTimer(w.opts.ShutdownTimeout)
	select {
	case <-drained:
		w.log("All jobs completed")
	case <-timer.C:
		w.log("Shutdown timeout reached, aborting remaining jobs")
		reason = "shutdown timeout"
		w.abortRemainingJobs()
	}
	timer.Stop()

	// 3. The worker heartbeat ran during the drain; stop it now.
	w.hbCancel()
	if w.heartbeatTicker != nil {
		w.heartbeatTicker.Stop()
	}
	w.hbWG.Wait()
	w.jobsCancel()

	// 4. Deregister LAST, once every job has been acknowledged. The server
	// releases whatever the worker still holds, so doing this earlier would put
	// a job that was about to complete back in the queue.
	if workerID != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := w.workers.Deregister(ctx, workerID); err != nil {
			w.log("Failed to deregister worker: %v", err)
		}
	}

	w.state.Store(StateStopped)
	w.emit(Event{
		Type:      EventWorkerStopped,
		Timestamp: time.Now(),
		Data:      WorkerStoppedData{WorkerID: workerID, Reason: reason},
	})
}

// abortRemainingJobs runs when the shutdown timeout expires. It cancels every
// remaining handler's context and fails the job with "Worker shutdown timeout",
// so it is retried by another worker instead of waiting for its lease to run
// out. Go cannot kill a goroutine: a handler that ignores its context keeps
// running, but the job goroutine stops waiting for it.
func (w *Worker) abortRemainingJobs() {
	// Claim the settlement of every remaining job BEFORE cancelling anything. A
	// cooperative handler returns "context canceled" the moment its context is
	// done; if it won the race its error, not "Worker shutdown timeout", would be
	// recorded as the failure.
	var ours, inFlight []*activeJob
	w.activeJobs.Range(func(_, value any) bool {
		aj := value.(*activeJob)
		if aj.beginSettle() {
			ours = append(ours, aj)
		} else {
			inFlight = append(inFlight, aj)
		}
		return true
	})

	w.jobsCancel()

	var wg sync.WaitGroup
	for _, aj := range ours {
		aj.abandonJob()
		wg.Add(1)
		go func() {
			defer wg.Done()
			w.reportFailure(aj, errShutdownTimeout, time.Since(aj.startTime))
		}()
	}
	for _, aj := range inFlight {
		aj.abandonJob()
		if aj.settleDone == nil {
			continue
		}
		// The handler (or the job timeout) is already acknowledging this job;
		// deregistering before that lands would release the job for a rerun.
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case <-aj.settleDone:
			case <-time.After(ackTimeout):
			}
		}()
	}
	wg.Wait()
}

// State returns the current worker state.
func (w *Worker) State() State {
	return w.state.Load().(State)
}

// WorkerID returns the registered worker ID.
func (w *Worker) WorkerID() string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.workerID
}

// ActiveJobCount returns the number of jobs currently being processed.
func (w *Worker) ActiveJobCount() int {
	return int(w.jobCount.Load())
}

// OnEvent registers an event handler.
func (w *Worker) OnEvent(handler EventHandler) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.eventHandlers = append(w.eventHandlers, handler)
}

// heartbeatInterval is how often leases and the worker are renewed. It is
// clamped to one second: LeaseDuration*HeartbeatFraction under a second used to
// truncate to zero, and time.NewTicker panics on a non-positive interval.
func (w *Worker) heartbeatInterval() time.Duration {
	d := time.Duration(float64(w.opts.LeaseDuration) * w.opts.HeartbeatFraction * float64(time.Second))
	if d < time.Second {
		d = time.Second
	}
	return d
}

// jobContext is the parent of every job context.
func (w *Worker) jobContext() context.Context {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if w.jobsCtx != nil {
		return w.jobsCtx
	}
	return context.Background()
}

// heartbeatContext is the context of worker-level heartbeats.
func (w *Worker) heartbeatContext() context.Context {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if w.hbCtx != nil {
		return w.hbCtx
	}
	return context.Background()
}

func (w *Worker) pollLoop() {
	defer w.pollWG.Done()

	// Do an immediate poll
	w.poll()

	for {
		select {
		case <-w.ctx.Done():
			return
		case <-w.pollTicker.C:
			w.poll()
		}
	}
}

func (w *Worker) poll() {
	if w.state.Load().(State) != StateRunning || w.ctx.Err() != nil {
		return
	}

	// Check capacity
	availableSlots := w.opts.Concurrency - int(w.jobCount.Load())
	if availableSlots <= 0 {
		return
	}

	w.mu.RLock()
	workerID := w.workerID
	w.mu.RUnlock()

	if workerID == "" {
		return
	}

	// Claim jobs
	ctx, cancel := context.WithTimeout(w.ctx, 10*time.Second)
	defer cancel()

	limit := availableSlots
	if limit > maxClaimBatch {
		// A claim over the API maximum is rejected (422) outright.
		limit = maxClaimBatch
	}
	leaseDuration := w.opts.LeaseDuration

	result, err := w.jobs.Claim(ctx, &resources.ClaimJobsRequest{
		QueueName:        w.opts.QueueName,
		WorkerID:         workerID,
		Limit:            &limit,
		LeaseDurationSec: &leaseDuration,
	})
	if err != nil {
		if w.ctx.Err() != nil {
			return // shutting down; an aborted claim is not a worker error
		}
		w.log("Poll failed: %v", err)
		w.emit(Event{
			Type:      EventWorkerError,
			Timestamp: time.Now(),
			Data:      WorkerErrorData{Error: err},
		})
		return
	}

	// Process claimed jobs. Jobs the claim returned are leased to this worker, so
	// they are run even if shutdown began while the claim was in flight.
	for _, job := range result.Jobs {
		w.processJob(job)
	}
}

func (w *Worker) processJob(job resources.ClaimedJob) {
	w.jobCount.Add(1)

	w.emit(Event{
		Type:      EventJobClaimed,
		Timestamp: time.Now(),
		Data:      JobClaimedData{JobID: job.ID, QueueName: job.QueueName},
	})

	// Create job context. It hangs off jobsCtx, not the claim context, so Stop
	// does not cancel a running handler before the shutdown timeout.
	jobCtx, jobCancel := context.WithCancel(w.jobContext())
	w.mu.RLock()
	workerID := w.workerID
	w.mu.RUnlock()
	aj := &activeJob{
		jobID:          job.ID,
		queueName:      job.QueueName,
		workerID:       workerID,
		retryCount:     job.RetryCount,
		maxRetries:     job.MaxRetries,
		timeoutSeconds: job.TimeoutSeconds,
		ctx:            jobCtx,
		cancel:         jobCancel,
		startTime:      time.Now(),
		leaseID:        job.LeaseID,
		settleDone:     make(chan struct{}),
		abandon:        make(chan struct{}),
	}
	// The heartbeat ticker exists before the job is published to activeJobs, so
	// shutdown never observes a half-built job.
	aj.heartbeat = time.NewTicker(w.heartbeatInterval())

	w.activeJobs.Store(job.ID, aj)

	// Start job heartbeat
	go w.jobHeartbeatLoop(aj)

	// Process in goroutine
	w.jobWG.Add(1)
	go func() {
		defer w.jobWG.Done()
		defer func() {
			w.activeJobs.CompareAndDelete(job.ID, aj)
			w.jobCount.Add(-1)
			jobCancel()
			aj.heartbeat.Stop()
		}()

		w.runJob(aj, job)
	}()
}

// handlerOutcome says how a handler call ended.
type handlerOutcome int

const (
	handlerReturned  handlerOutcome = iota // the handler returned (a result, an error, or a panic)
	handlerTimedOut                        // the job exceeded timeout_seconds
	handlerAbandoned                       // shutdown gave up on the job
)

type handlerResult struct {
	result map[string]any
	err    error
}

func (w *Worker) runJob(aj *activeJob, job resources.ClaimedJob) {
	w.emit(Event{
		Type:      EventJobStarted,
		Timestamp: time.Now(),
		Data:      JobStartedData{JobID: job.ID, QueueName: job.QueueName},
	})

	// Payload keeps the map view for object payloads; RawPayload carries any JSON
	// value so array/string/number payloads reach the handler too.
	payload, _ := job.Payload.(map[string]any)
	// Build job context
	jctx := &JobContext{
		Context:    aj.ctx,
		JobID:      job.ID,
		QueueName:  job.QueueName,
		Payload:    payload,
		RawPayload: job.Payload,
		RetryCount: job.RetryCount,
		MaxRetries: job.MaxRetries,
		workerID:   aj.workerID,
		worker:     w,
		Progress: func(float64, string) error {
			return &resources.UnsupportedError{
				Operation: "JobContext.Progress",
				Reason:    "the API has no job progress route; return progress in the job result or report it with JobContext.Log",
			}
		},
		Log: func(level string, message string, meta map[string]any) {
			w.log("[job:%s] [%s] %s %v", job.ID, level, message, meta)
		},
	}

	// Call handler
	w.mu.RLock()
	handler := w.handler
	w.mu.RUnlock()

	res, outcome := w.callHandler(aj, handler, jctx)
	duration := time.Since(aj.startTime)

	// A lease-expired heartbeat already canceled and signaled this exact
	// execution. Never settle it afterward: doing so would make a stale
	// complete/fail request and could emit a duplicate lease-loss event.
	if aj.leaseLost.Load() {
		return
	}

	switch outcome {
	case handlerAbandoned:
		// Shutdown timed out and already failed this job.
		return
	case handlerTimedOut:
		w.emit(Event{
			Type:      EventJobTimeout,
			Timestamp: time.Now(),
			Data: JobTimeoutData{
				JobID:          aj.jobID,
				QueueName:      aj.queueName,
				TimeoutSeconds: aj.timeoutSeconds,
				Duration:       duration,
			},
		})
		w.failJob(aj, errJobTimeout, duration)
	default:
		if res.err != nil {
			// Job failed
			w.failJob(aj, res.err, duration)
		} else {
			// Job completed
			w.completeJob(aj, res.result, duration)
		}
	}
}

// callHandler runs the handler and returns when it returns, when the job's
// timeout_seconds elapses, or when shutdown abandons the job.
//
// A panic in the handler is recovered and becomes a job failure carrying the
// panic text: an unrecovered panic on this goroutine would take down the whole
// process and every other job in it, and the poisoned job would crash the next
// worker that claims it too.
//
// On a timeout the job context is canceled, which also stops lease renewal: a
// hung handler must not keep the lease alive forever. The handler goroutine is
// left to finish on its own (Go cannot stop it); its late result is dropped.
func (w *Worker) callHandler(aj *activeJob, handler JobHandler, jctx *JobContext) (handlerResult, handlerOutcome) {
	done := make(chan handlerResult, 1)
	go func() {
		var r handlerResult
		returned := false
		defer func() {
			if rec := recover(); rec != nil {
				w.log("Handler panic in job %s: %v\n%s", aj.jobID, rec, debug.Stack())
				r = handlerResult{err: fmt.Errorf("panic: %v", rec)}
			} else if !returned {
				// runtime.Goexit (for example t.FailNow in a test handler) unwinds
				// without a panic or a return.
				r = handlerResult{err: errors.New("handler exited without returning")}
			}
			done <- r
		}()
		r.result, r.err = handler(jctx)
		returned = true
	}()

	var timeout <-chan time.Time
	if aj.timeoutSeconds > 0 {
		timer := time.NewTimer(time.Duration(aj.timeoutSeconds) * time.Second)
		defer timer.Stop()
		timeout = timer.C
	}

	select {
	case r := <-done:
		return r, handlerReturned
	case <-timeout:
		w.log("Job %s exceeded its %ds timeout", aj.jobID, aj.timeoutSeconds)
		aj.cancel()
		return handlerResult{}, handlerTimedOut
	case <-aj.abandon:
		return handlerResult{}, handlerAbandoned
	}
}

func (w *Worker) completeJob(aj *activeJob, result map[string]any, duration time.Duration) {
	if aj.leaseLost.Load() || !aj.beginSettle() {
		return
	}

	// Background, not a job or worker context: a result must still be sent after
	// Stop has begun.
	ctx, cancel := context.WithTimeout(context.Background(), ackTimeout)
	defer cancel()

	err := w.jobs.Complete(ctx, aj.jobID, &resources.CompleteJobRequest{
		WorkerID: aj.workerID,
		Result:   result,
		LeaseID:  aj.leaseID,
	})
	// Signaled as soon as the request is over, before event callbacks run, so a
	// callback that calls Stop cannot make shutdown wait on its own ack.
	aj.endSettle()
	if err != nil {
		// A retried ack whose first attempt actually landed meets "Job is already
		// completed". The work is recorded, which is all that matters.
		if !isAlreadySettled(err) {
			w.handleExecutionError(aj, "complete", err)
			return
		}
		w.log("Job %s was already completed by an earlier attempt", aj.jobID)
	}

	w.emit(Event{
		Type:      EventJobCompleted,
		Timestamp: time.Now(),
		Data: JobCompletedData{
			JobID:     aj.jobID,
			QueueName: aj.queueName,
			Result:    result,
			Duration:  duration,
		},
	})

	w.log("Job completed: id=%s duration=%v", aj.jobID, duration)
}

func (w *Worker) failJob(aj *activeJob, jobErr error, duration time.Duration) {
	if aj.leaseLost.Load() || !aj.beginSettle() {
		return
	}
	w.reportFailure(aj, jobErr, duration)
}

// reportFailure sends the failure for a job whose settlement the caller already
// claimed with beginSettle, and ends that settlement.
func (w *Worker) reportFailure(aj *activeJob, jobErr error, duration time.Duration) {
	if aj.leaseLost.Load() {
		aj.endSettle()
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), ackTimeout)
	defer cancel()

	// FailWithResponse coerces the text: an empty message or one over 2048
	// characters is rejected by the API and would leave the job processing.
	response, err := w.jobs.FailWithResponse(ctx, aj.jobID, &resources.FailJobRequest{
		WorkerID: aj.workerID,
		Error:    jobErr.Error(),
		LeaseID:  aj.leaseID,
	})
	aj.endSettle()
	if err != nil {
		if !isAlreadySettled(err) {
			w.handleExecutionError(aj, "fail", err)
			return
		}
		// A retried ack whose first attempt landed; the disposition is unknown.
		w.log("Job %s was already failed by an earlier attempt", aj.jobID)
		response = &resources.FailJobResponse{}
	}

	willRetry := aj.retryCount < aj.maxRetries
	if response.WillRetry != nil {
		willRetry = *response.WillRetry
	}

	w.emit(Event{
		Type:      EventJobFailed,
		Timestamp: time.Now(),
		Data: JobFailedData{
			JobID:     aj.jobID,
			QueueName: aj.queueName,
			Error:     jobErr,
			Duration:  duration,
			WillRetry: willRetry,
		},
	})

	w.log("Job failed: id=%s error=%v duration=%v", aj.jobID, jobErr, duration)
}

func (w *Worker) jobHeartbeatLoop(aj *activeJob) {
	for {
		select {
		case <-aj.ctx.Done():
			return
		case <-aj.heartbeat.C:
			if !w.renewJobLease(aj) {
				return
			}
		}
	}
}

func (w *Worker) renewJobLease(aj *activeJob) bool {
	ctx, cancel := context.WithTimeout(aj.ctx, renewTimeout)
	defer cancel()

	if _, err := w.jobs.RenewLease(ctx, aj.jobID, &resources.RenewLeaseRequest{
		WorkerID:         aj.workerID,
		LeaseDurationSec: w.opts.LeaseDuration,
		LeaseID:          aj.leaseID,
	}); err != nil {
		w.handleExecutionError(aj, "renew lease", err)
		return !isLeaseExpired(err)
	}

	w.emit(Event{
		Type:      EventJobHeartbeat,
		Timestamp: time.Now(),
		Data:      map[string]string{"job_id": aj.jobID},
	})
	return true
}

func (w *Worker) handleExecutionError(aj *activeJob, operation string, err error) {
	w.log("Failed to %s for job %s: %v", operation, aj.jobID, err)
	if isLeaseExpired(err) {
		if !aj.leaseLost.CompareAndSwap(false, true) {
			return
		}
		if aj.cancel != nil {
			aj.cancel()
		}
		if aj.heartbeat != nil {
			aj.heartbeat.Stop()
		}
		w.emit(Event{
			Type:      EventJobLeaseLost,
			Timestamp: time.Now(),
			Data: JobLeaseLostData{
				JobID:     aj.jobID,
				QueueName: aj.queueName,
				LeaseID:   leaseIDValue(aj.leaseID),
				Operation: operation,
				Error:     err,
			},
		})
		return
	}

	w.emit(Event{
		Type:      EventWorkerError,
		Timestamp: time.Now(),
		Data: WorkerErrorData{
			Error: fmt.Errorf("failed to %s job %s: %w", operation, aj.jobID, err),
		},
	})
}

func isLeaseExpired(err error) bool {
	var apiErr *httpx.APIError
	return errors.As(err, &apiErr) && strings.EqualFold(apiErr.Code, "LEASE_EXPIRED")
}

// isAlreadySettled recognizes the 409 "Job is already <status>" the API sends
// when an acknowledgement is repeated for a job this worker already settled. It
// is what a retried complete/fail meets when the first attempt reached the
// server and only the response was lost.
func isAlreadySettled(err error) bool {
	var conflict *httpx.ConflictError
	return errors.As(err, &conflict) && !isLeaseExpired(err) && strings.HasPrefix(conflict.Message, "Job is already ")
}

func leaseIDValue(leaseID *string) string {
	if leaseID == nil {
		return ""
	}
	return *leaseID
}

func (w *Worker) workerHeartbeatLoop() {
	defer w.hbWG.Done()

	ctx := w.heartbeatContext()
	for {
		select {
		case <-ctx.Done():
			return
		case <-w.heartbeatTicker.C:
			// select picks at random when the tick and the shutdown arrive
			// together; do not start a heartbeat that is already canceled.
			if ctx.Err() != nil {
				return
			}
			w.sendWorkerHeartbeat()
		}
	}
}

func (w *Worker) sendWorkerHeartbeat() {
	ctx, cancel := context.WithTimeout(w.heartbeatContext(), renewTimeout)
	defer cancel()

	w.mu.RLock()
	workerID := w.workerID
	w.mu.RUnlock()

	if workerID == "" {
		return
	}

	// Map the worker's internal state to the backend heartbeat status enum,
	// which only accepts healthy/degraded/draining/offline. Sending "active"
	// or "stopping" is rejected with a 400 VALIDATION_ERROR.
	var status string
	switch w.state.Load().(State) {
	case StateRunning:
		status = string(resources.WorkerStatusHealthy)
	case StateStopping:
		status = string(resources.WorkerStatusDraining)
	default:
		status = string(resources.WorkerStatusOffline)
	}

	currentJobs := int(w.jobCount.Load())
	if err := w.workers.Heartbeat(ctx, workerID, &resources.WorkerHeartbeatRequest{
		CurrentJobs: currentJobs,
		Status:      &status,
	}); err != nil {
		// Shutdown cancels the heartbeat context once the jobs have drained,
		// which aborts a heartbeat in flight. That is the intended end of the
		// heartbeat loop, not a failure worth reporting.
		if w.heartbeatContext().Err() != nil || errors.Is(err, context.Canceled) {
			return
		}
		w.log("Failed to send worker heartbeat: %v", err)
	} else {
		w.emit(Event{
			Type:      EventWorkerHeartbeat,
			Timestamp: time.Now(),
			Data:      map[string]string{"worker_id": workerID},
		})
	}
}

func (w *Worker) emit(event Event) {
	w.mu.RLock()
	handlers := make([]EventHandler, len(w.eventHandlers))
	copy(handlers, w.eventHandlers)
	w.mu.RUnlock()

	for _, handler := range handlers {
		func() {
			defer func() {
				if r := recover(); r != nil {
					w.log("Event handler panic: %v", r)
				}
			}()
			handler(event)
		}()
	}
}

func (w *Worker) log(format string, args ...any) {
	if w.opts.Logger != nil {
		w.opts.Logger(format, args...)
	} else if w.opts.Debug {
		fmt.Printf("[spooled-worker] "+format+"\n", args...)
	}
}
