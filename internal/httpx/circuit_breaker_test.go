package httpx

import (
	"testing"
	"time"
)

func TestCircuitBreaker_InitialState(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		FailureThreshold: 5,
		SuccessThreshold: 3,
		Timeout:          30 * time.Second,
	})

	if cb.State() != CircuitClosed {
		t.Errorf("Expected initial state to be Closed, got %v", cb.State())
	}

	if !cb.Allow() {
		t.Error("Expected Allow() to return true in Closed state")
	}
}

func TestCircuitBreaker_OpensAfterFailures(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		FailureThreshold: 3,
		SuccessThreshold: 2,
		Timeout:          30 * time.Second,
	})

	// Record failures below threshold
	cb.RecordFailure()
	cb.RecordFailure()

	if cb.State() != CircuitClosed {
		t.Errorf("Expected state to be Closed after 2 failures, got %v", cb.State())
	}

	// Third failure should open the circuit
	cb.RecordFailure()

	if cb.State() != CircuitOpen {
		t.Errorf("Expected state to be Open after 3 failures, got %v", cb.State())
	}

	if cb.Allow() {
		t.Error("Expected Allow() to return false in Open state")
	}
}

func TestCircuitBreaker_SuccessResetsFailureCount(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		FailureThreshold: 3,
		SuccessThreshold: 2,
		Timeout:          30 * time.Second,
	})

	// Record some failures
	cb.RecordFailure()
	cb.RecordFailure()

	// Success should reset failure count
	cb.RecordSuccess()

	metrics := cb.Metrics()
	if metrics.FailureCount != 0 {
		t.Errorf("Expected failure count to be reset to 0, got %d", metrics.FailureCount)
	}

	// Need 3 more failures to open circuit
	cb.RecordFailure()
	cb.RecordFailure()
	if cb.State() != CircuitClosed {
		t.Error("Expected state to still be Closed")
	}

	cb.RecordFailure()
	if cb.State() != CircuitOpen {
		t.Error("Expected state to be Open after 3 consecutive failures")
	}
}

func TestCircuitBreaker_TransitionsToHalfOpen(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		FailureThreshold: 1,
		SuccessThreshold: 1,
		Timeout:          10 * time.Millisecond,
	})

	// Open the circuit
	cb.RecordFailure()
	if cb.State() != CircuitOpen {
		t.Errorf("Expected state to be Open, got %v", cb.State())
	}

	// Wait for timeout
	time.Sleep(20 * time.Millisecond)

	// Allow() should transition to HalfOpen
	if !cb.Allow() {
		t.Error("Expected Allow() to return true and transition to HalfOpen")
	}

	if cb.State() != CircuitHalfOpen {
		t.Errorf("Expected state to be HalfOpen, got %v", cb.State())
	}
}

func TestCircuitBreaker_HalfOpenClosesOnSuccess(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		FailureThreshold: 1,
		SuccessThreshold: 2,
		Timeout:          10 * time.Millisecond,
	})

	// Open the circuit
	cb.RecordFailure()

	// Wait for timeout and transition to half-open
	time.Sleep(20 * time.Millisecond)
	cb.Allow()

	if cb.State() != CircuitHalfOpen {
		t.Errorf("Expected state to be HalfOpen, got %v", cb.State())
	}

	// First success
	cb.RecordSuccess()
	if cb.State() != CircuitHalfOpen {
		t.Errorf("Expected state to still be HalfOpen after 1 success, got %v", cb.State())
	}

	// Second success should close the circuit
	cb.RecordSuccess()
	if cb.State() != CircuitClosed {
		t.Errorf("Expected state to be Closed after 2 successes, got %v", cb.State())
	}
}

func TestCircuitBreaker_HalfOpenReopensOnFailure(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		FailureThreshold: 1,
		SuccessThreshold: 3,
		Timeout:          10 * time.Millisecond,
	})

	// Open the circuit
	cb.RecordFailure()

	// Wait for timeout and transition to half-open
	time.Sleep(20 * time.Millisecond)
	cb.Allow()

	if cb.State() != CircuitHalfOpen {
		t.Errorf("Expected state to be HalfOpen, got %v", cb.State())
	}

	// Failure in half-open should reopen
	cb.RecordFailure()

	if cb.State() != CircuitOpen {
		t.Errorf("Expected state to be Open after failure in HalfOpen, got %v", cb.State())
	}
}

func TestCircuitBreaker_Reset(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		FailureThreshold: 1,
		SuccessThreshold: 1,
		Timeout:          30 * time.Second,
	})

	// Open the circuit
	cb.RecordFailure()
	if cb.State() != CircuitOpen {
		t.Errorf("Expected state to be Open, got %v", cb.State())
	}

	// Reset should close the circuit
	cb.Reset()

	if cb.State() != CircuitClosed {
		t.Errorf("Expected state to be Closed after Reset, got %v", cb.State())
	}

	metrics := cb.Metrics()
	if metrics.FailureCount != 0 || metrics.SuccessCount != 0 {
		t.Errorf("Expected counts to be reset, got failure=%d, success=%d", metrics.FailureCount, metrics.SuccessCount)
	}
}

func TestCircuitBreaker_Defaults(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{})

	if cb.failureThreshold != 5 {
		t.Errorf("Expected default failureThreshold=5, got %d", cb.failureThreshold)
	}
	if cb.successThreshold != 3 {
		t.Errorf("Expected default successThreshold=3, got %d", cb.successThreshold)
	}
	if cb.timeout != 30*time.Second {
		t.Errorf("Expected default timeout=30s, got %v", cb.timeout)
	}
}

func TestCircuitState_String(t *testing.T) {
	tests := []struct {
		state CircuitState
		want  string
	}{
		{CircuitClosed, "closed"},
		{CircuitOpen, "open"},
		{CircuitHalfOpen, "half-open"},
		{CircuitState(99), "unknown"},
	}

	for _, tt := range tests {
		got := tt.state.String()
		if got != tt.want {
			t.Errorf("CircuitState(%d).String() = %q, want %q", tt.state, got, tt.want)
		}
	}
}
