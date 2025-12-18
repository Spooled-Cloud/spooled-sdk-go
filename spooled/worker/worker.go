package worker

import (
	"context"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/spooled-cloud/spooled-sdk-go/spooled/resources"
)

// activeJob tracks an in-progress job.
type activeJob struct {
	jobID     string
	ctx       context.Context
	cancel    context.CancelFunc
	startTime time.Time
	heartbeat *time.Ticker
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

	pollTicker       *time.Ticker
	heartbeatTicker  *time.Ticker
	eventHandlers    []EventHandler

	mu       sync.RWMutex
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	stopOnce sync.Once
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
		hostname, _ := os.Hostname()
		opts.Hostname = hostname
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

// Start starts the worker.
func (w *Worker) Start(ctx context.Context) error {
	w.mu.Lock()
	if w.handler == nil {
		w.mu.Unlock()
		return fmt.Errorf("no job handler registered; call Process() first")
	}

	if w.state.Load().(State) != StateIdle {
		w.mu.Unlock()
		return fmt.Errorf("worker already started")
	}

	w.state.Store(StateStarting)
	w.ctx, w.cancel = context.WithCancel(ctx)
	w.mu.Unlock()

	// Register worker with API
	concurrency := w.opts.Concurrency
	version := w.opts.Version
	workerType := w.opts.WorkerType
	metadata := make(map[string]any)
	for k, v := range w.opts.Metadata {
		metadata[k] = v
	}

	resp, err := w.workers.Register(ctx, &resources.RegisterWorkerRequest{
		QueueName:      w.opts.QueueName,
		Hostname:       w.opts.Hostname,
		MaxConcurrency: &concurrency,
		Version:        &version,
		WorkerType:     &workerType,
		Metadata:       metadata,
	})
	if err != nil {
		w.state.Store(StateError)
		return fmt.Errorf("failed to register worker: %w", err)
	}

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
	w.pollTicker = time.NewTicker(w.opts.PollInterval)
	w.wg.Add(1)
	go w.pollLoop()

	// Start worker heartbeat
	heartbeatInterval := time.Duration(float64(w.opts.LeaseDuration)*w.opts.HeartbeatFraction) * time.Second
	w.heartbeatTicker = time.NewTicker(heartbeatInterval)
	w.wg.Add(1)
	go w.workerHeartbeatLoop()

	w.log("Worker started: id=%s queue=%s", w.workerID, w.opts.QueueName)
	return nil
}

// Stop gracefully stops the worker.
func (w *Worker) Stop() error {
	var err error
	w.stopOnce.Do(func() {
		err = w.doStop()
	})
	return err
}

func (w *Worker) doStop() error {
	w.mu.Lock()
	state := w.state.Load().(State)
	if state != StateRunning {
		w.mu.Unlock()
		return nil
	}

	w.state.Store(StateStopping)
	workerID := w.workerID
	w.mu.Unlock()

	w.log("Stopping worker: id=%s", workerID)

	// Stop polling
	if w.pollTicker != nil {
		w.pollTicker.Stop()
	}
	if w.heartbeatTicker != nil {
		w.heartbeatTicker.Stop()
	}

	// Cancel all active jobs
	w.activeJobs.Range(func(key, value any) bool {
		aj := value.(*activeJob)
		aj.cancel()
		if aj.heartbeat != nil {
			aj.heartbeat.Stop()
		}
		return true
	})

	// Cancel worker context
	if w.cancel != nil {
		w.cancel()
	}

	// Wait for goroutines with timeout
	done := make(chan struct{})
	go func() {
		w.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		w.log("All jobs completed")
	case <-time.After(w.opts.ShutdownTimeout):
		w.log("Shutdown timeout reached, forcing stop")
	}

	// Deregister worker
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
		Data:      WorkerStoppedData{WorkerID: workerID, Reason: "graceful shutdown"},
	})

	return nil
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

func (w *Worker) pollLoop() {
	defer w.wg.Done()

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
	if w.state.Load().(State) != StateRunning {
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
	leaseDuration := w.opts.LeaseDuration

	result, err := w.jobs.Claim(ctx, &resources.ClaimJobsRequest{
		QueueName:        w.opts.QueueName,
		WorkerID:         workerID,
		Limit:            &limit,
		LeaseDurationSec: &leaseDuration,
	})
	if err != nil {
		w.log("Poll failed: %v", err)
		w.emit(Event{
			Type:      EventWorkerError,
			Timestamp: time.Now(),
			Data:      WorkerErrorData{Error: err},
		})
		return
	}

	// Process claimed jobs
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

	// Create job context
	jobCtx, jobCancel := context.WithCancel(w.ctx)
	aj := &activeJob{
		jobID:     job.ID,
		ctx:       jobCtx,
		cancel:    jobCancel,
		startTime: time.Now(),
	}

	w.activeJobs.Store(job.ID, aj)

	// Start job heartbeat
	heartbeatInterval := time.Duration(float64(w.opts.LeaseDuration)*w.opts.HeartbeatFraction) * time.Second
	aj.heartbeat = time.NewTicker(heartbeatInterval)
	go w.jobHeartbeatLoop(aj)

	// Process in goroutine
	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		defer func() {
			w.activeJobs.Delete(job.ID)
			w.jobCount.Add(-1)
			jobCancel()
			if aj.heartbeat != nil {
				aj.heartbeat.Stop()
			}
		}()

		w.emit(Event{
			Type:      EventJobStarted,
			Timestamp: time.Now(),
			Data:      JobStartedData{JobID: job.ID, QueueName: job.QueueName},
		})

		// Build job context
		jctx := &JobContext{
			Context:    jobCtx,
			JobID:      job.ID,
			QueueName:  job.QueueName,
			Payload:    job.Payload,
			RetryCount: job.RetryCount,
			MaxRetries: job.MaxRetries,
			workerID:   w.workerID,
			worker:     w,
			Progress: func(percent float64, message string) error {
				return w.updateProgress(job.ID, percent, message)
			},
			Log: func(level string, message string, meta map[string]any) {
				w.log("[job:%s] [%s] %s %v", job.ID, level, message, meta)
			},
		}

		// Call handler
		w.mu.RLock()
		handler := w.handler
		w.mu.RUnlock()

		result, err := handler(jctx)
		duration := time.Since(aj.startTime)

		if err != nil {
			// Job failed
			w.failJob(job.ID, err, duration)
		} else {
			// Job completed
			w.completeJob(job.ID, result, duration)
		}
	}()
}

func (w *Worker) completeJob(jobID string, result map[string]any, duration time.Duration) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	w.mu.RLock()
	workerID := w.workerID
	w.mu.RUnlock()

	if err := w.jobs.Complete(ctx, jobID, &resources.CompleteJobRequest{
		WorkerID: workerID,
		Result:   result,
	}); err != nil {
		w.log("Failed to complete job %s: %v", jobID, err)
	}

	w.emit(Event{
		Type:      EventJobCompleted,
		Timestamp: time.Now(),
		Data: JobCompletedData{
			JobID:    jobID,
			Result:   result,
			Duration: duration,
		},
	})

	w.log("Job completed: id=%s duration=%v", jobID, duration)
}

func (w *Worker) failJob(jobID string, jobErr error, duration time.Duration) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	w.mu.RLock()
	workerID := w.workerID
	w.mu.RUnlock()

	if err := w.jobs.Fail(ctx, jobID, &resources.FailJobRequest{
		WorkerID: workerID,
		Error:    jobErr.Error(),
	}); err != nil {
		w.log("Failed to fail job %s: %v", jobID, err)
	}

	w.emit(Event{
		Type:      EventJobFailed,
		Timestamp: time.Now(),
		Data: JobFailedData{
			JobID:    jobID,
			Error:    jobErr,
			Duration: duration,
		},
	})

	w.log("Job failed: id=%s error=%v duration=%v", jobID, jobErr, duration)
}

func (w *Worker) updateProgress(jobID string, percent float64, message string) error {
	ctx, cancel := context.WithTimeout(w.ctx, 5*time.Second)
	defer cancel()

	if err := w.jobs.UpdateProgress(ctx, jobID, &resources.UpdateProgressRequest{
		Progress: percent,
		Message:  message,
	}); err != nil {
		return err
	}

	w.emit(Event{
		Type:      EventJobProgress,
		Timestamp: time.Now(),
		Data: JobProgressData{
			JobID:   jobID,
			Percent: percent,
			Message: message,
		},
	})

	return nil
}

func (w *Worker) jobHeartbeatLoop(aj *activeJob) {
	for {
		select {
		case <-aj.ctx.Done():
			return
		case <-aj.heartbeat.C:
			w.renewJobLease(aj.jobID)
		}
	}
}

func (w *Worker) renewJobLease(jobID string) {
	ctx, cancel := context.WithTimeout(w.ctx, 5*time.Second)
	defer cancel()

	w.mu.RLock()
	workerID := w.workerID
	w.mu.RUnlock()

	if _, err := w.jobs.RenewLease(ctx, jobID, &resources.RenewLeaseRequest{
		WorkerID:         workerID,
		LeaseDurationSec: w.opts.LeaseDuration,
	}); err != nil {
		w.log("Failed to renew lease for job %s: %v", jobID, err)
	} else {
		w.emit(Event{
			Type:      EventJobHeartbeat,
			Timestamp: time.Now(),
			Data:      map[string]string{"job_id": jobID},
		})
	}
}

func (w *Worker) workerHeartbeatLoop() {
	defer w.wg.Done()

	for {
		select {
		case <-w.ctx.Done():
			return
		case <-w.heartbeatTicker.C:
			w.sendWorkerHeartbeat()
		}
	}
}

func (w *Worker) sendWorkerHeartbeat() {
	ctx, cancel := context.WithTimeout(w.ctx, 5*time.Second)
	defer cancel()

	w.mu.RLock()
	workerID := w.workerID
	w.mu.RUnlock()

	if workerID == "" {
		return
	}

	status := "active"
	if w.state.Load().(State) != StateRunning {
		status = "stopping"
	}

	currentJobs := int(w.jobCount.Load())
	if err := w.workers.Heartbeat(ctx, workerID, &resources.WorkerHeartbeatRequest{
		CurrentJobs: currentJobs,
		Status:      &status,
	}); err != nil {
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
