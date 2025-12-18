package httpx

import (
	"sync"
	"time"
)

// CircuitState represents the state of a circuit breaker.
type CircuitState int

const (
	// CircuitClosed - normal operation, requests are allowed
	CircuitClosed CircuitState = iota
	// CircuitOpen - circuit is open, requests are blocked
	CircuitOpen
	// CircuitHalfOpen - testing if circuit can be closed
	CircuitHalfOpen
)

// String returns the string representation of a circuit state.
func (s CircuitState) String() string {
	switch s {
	case CircuitClosed:
		return "closed"
	case CircuitOpen:
		return "open"
	case CircuitHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// CircuitBreaker implements the circuit breaker pattern.
type CircuitBreaker struct {
	mu               sync.RWMutex
	state            CircuitState
	failureCount     int
	successCount     int
	failureThreshold int
	successThreshold int
	timeout          time.Duration
	lastStateChange  time.Time
}

// NewCircuitBreaker creates a new circuit breaker.
func NewCircuitBreaker(cfg CircuitBreakerConfig) *CircuitBreaker {
	if cfg.FailureThreshold == 0 {
		cfg.FailureThreshold = 5
	}
	if cfg.SuccessThreshold == 0 {
		cfg.SuccessThreshold = 3
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Second
	}

	return &CircuitBreaker{
		state:            CircuitClosed,
		failureThreshold: cfg.FailureThreshold,
		successThreshold: cfg.SuccessThreshold,
		timeout:          cfg.Timeout,
		lastStateChange:  time.Now(),
	}
}

// Allow checks if a request is allowed through the circuit breaker.
func (cb *CircuitBreaker) Allow() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case CircuitClosed:
		return true
	case CircuitOpen:
		// Check if timeout has passed
		if time.Since(cb.lastStateChange) >= cb.timeout {
			cb.transitionTo(CircuitHalfOpen)
			return true
		}
		return false
	case CircuitHalfOpen:
		// Allow one test request
		return true
	default:
		return true
	}
}

// RecordSuccess records a successful request.
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case CircuitClosed:
		// Reset failure count on success
		cb.failureCount = 0
	case CircuitHalfOpen:
		cb.successCount++
		if cb.successCount >= cb.successThreshold {
			cb.transitionTo(CircuitClosed)
		}
	}
}

// RecordFailure records a failed request.
func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case CircuitClosed:
		cb.failureCount++
		if cb.failureCount >= cb.failureThreshold {
			cb.transitionTo(CircuitOpen)
		}
	case CircuitHalfOpen:
		// Any failure in half-open state reopens the circuit
		cb.transitionTo(CircuitOpen)
	}
}

// State returns the current state of the circuit breaker.
func (cb *CircuitBreaker) State() CircuitState {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// transitionTo transitions to a new state (must be called with lock held).
func (cb *CircuitBreaker) transitionTo(state CircuitState) {
	cb.state = state
	cb.failureCount = 0
	cb.successCount = 0
	cb.lastStateChange = time.Now()
}

// Reset resets the circuit breaker to its initial state.
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.transitionTo(CircuitClosed)
}

// Metrics returns current circuit breaker metrics.
func (cb *CircuitBreaker) Metrics() CircuitBreakerMetrics {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return CircuitBreakerMetrics{
		State:           cb.state,
		FailureCount:    cb.failureCount,
		SuccessCount:    cb.successCount,
		LastStateChange: cb.lastStateChange,
	}
}

// CircuitBreakerMetrics holds circuit breaker metrics.
type CircuitBreakerMetrics struct {
	State           CircuitState
	FailureCount    int
	SuccessCount    int
	LastStateChange time.Time
}


