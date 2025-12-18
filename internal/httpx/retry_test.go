package httpx

import (
	"testing"
	"time"
)

func TestRetryPolicy_Delay(t *testing.T) {
	tests := []struct {
		name     string
		config   RetryConfig
		attempt  int
		minDelay time.Duration
		maxDelay time.Duration
	}{
		{
			name: "first retry",
			config: RetryConfig{
				BaseDelay: 1 * time.Second,
				MaxDelay:  30 * time.Second,
				Factor:    2.0,
				Jitter:    false,
			},
			attempt:  0,
			minDelay: 1 * time.Second,
			maxDelay: 1 * time.Second,
		},
		{
			name: "second retry",
			config: RetryConfig{
				BaseDelay: 1 * time.Second,
				MaxDelay:  30 * time.Second,
				Factor:    2.0,
				Jitter:    false,
			},
			attempt:  1,
			minDelay: 2 * time.Second,
			maxDelay: 2 * time.Second,
		},
		{
			name: "third retry",
			config: RetryConfig{
				BaseDelay: 1 * time.Second,
				MaxDelay:  30 * time.Second,
				Factor:    2.0,
				Jitter:    false,
			},
			attempt:  2,
			minDelay: 4 * time.Second,
			maxDelay: 4 * time.Second,
		},
		{
			name: "capped at max delay",
			config: RetryConfig{
				BaseDelay: 1 * time.Second,
				MaxDelay:  5 * time.Second,
				Factor:    2.0,
				Jitter:    false,
			},
			attempt:  10,
			minDelay: 5 * time.Second,
			maxDelay: 5 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy := NewRetryPolicy(tt.config)
			delay := policy.Delay(tt.attempt)
			if delay < tt.minDelay || delay > tt.maxDelay {
				t.Errorf("Delay(%d) = %v, want between %v and %v", tt.attempt, delay, tt.minDelay, tt.maxDelay)
			}
		})
	}
}

func TestRetryPolicy_DelayWithJitter(t *testing.T) {
	policy := NewRetryPolicy(RetryConfig{
		BaseDelay: 1 * time.Second,
		MaxDelay:  30 * time.Second,
		Factor:    2.0,
		Jitter:    false, // Use DelayWithJitter for deterministic testing
	})

	tests := []struct {
		name         string
		attempt      int
		jitterFactor float64
		expected     time.Duration
	}{
		{
			name:         "no jitter factor",
			attempt:      0,
			jitterFactor: 1.0,
			expected:     1 * time.Second,
		},
		{
			name:         "minimum jitter factor",
			attempt:      0,
			jitterFactor: 0.5,
			expected:     500 * time.Millisecond,
		},
		{
			name:         "maximum jitter factor",
			attempt:      0,
			jitterFactor: 1.5,
			expected:     1500 * time.Millisecond,
		},
		{
			name:         "jitter on second attempt",
			attempt:      1,
			jitterFactor: 0.75,
			expected:     1500 * time.Millisecond, // 2s * 0.75
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			delay := policy.DelayWithJitter(tt.attempt, tt.jitterFactor)
			if delay != tt.expected {
				t.Errorf("DelayWithJitter(%d, %f) = %v, want %v", tt.attempt, tt.jitterFactor, delay, tt.expected)
			}
		})
	}
}

func TestRetryPolicy_DelayWithJitterVariance(t *testing.T) {
	policy := NewRetryPolicy(RetryConfig{
		BaseDelay: 1 * time.Second,
		MaxDelay:  30 * time.Second,
		Factor:    2.0,
		Jitter:    true,
	})

	// With jitter enabled, delays should vary
	delays := make([]time.Duration, 10)
	for i := 0; i < 10; i++ {
		delays[i] = policy.Delay(0)
	}

	// Check that we have some variance (not all delays are the same)
	allSame := true
	for i := 1; i < len(delays); i++ {
		if delays[i] != delays[0] {
			allSame = false
			break
		}
	}

	if allSame {
		t.Error("Expected delays to vary with jitter enabled")
	}

	// Check that all delays are within expected range (0.5x to 1.5x base)
	for i, d := range delays {
		if d < 500*time.Millisecond || d > 1500*time.Millisecond {
			t.Errorf("Delay[%d] = %v, want between 500ms and 1500ms", i, d)
		}
	}
}

func TestRetryPolicy_ShouldRetry(t *testing.T) {
	policy := NewRetryPolicy(RetryConfig{
		MaxRetries: 3,
	})

	tests := []struct {
		attempt int
		want    bool
	}{
		{0, true},
		{1, true},
		{2, true},
		{3, false},
		{4, false},
	}

	for _, tt := range tests {
		got := policy.ShouldRetry(tt.attempt)
		if got != tt.want {
			t.Errorf("ShouldRetry(%d) = %v, want %v", tt.attempt, got, tt.want)
		}
	}
}

func TestRetryPolicy_Defaults(t *testing.T) {
	policy := NewRetryPolicy(RetryConfig{})

	// MaxRetries isn't defaulted in NewRetryPolicy (0 means no retries is valid)
	if policy.MaxRetries != 0 {
		t.Errorf("Expected MaxRetries=0 (not defaulted), got %d", policy.MaxRetries)
	}
	if policy.BaseDelay != 1*time.Second {
		t.Errorf("Expected default BaseDelay=1s, got %v", policy.BaseDelay)
	}
	if policy.MaxDelay != 30*time.Second {
		t.Errorf("Expected default MaxDelay=30s, got %v", policy.MaxDelay)
	}
	if policy.Factor != 2.0 {
		t.Errorf("Expected default Factor=2.0, got %f", policy.Factor)
	}
}

func TestDefaultRetryConfig(t *testing.T) {
	cfg := DefaultRetryConfig()
	if cfg.MaxRetries != 3 {
		t.Errorf("Expected MaxRetries=3, got %d", cfg.MaxRetries)
	}
	if cfg.BaseDelay != 1*time.Second {
		t.Errorf("Expected BaseDelay=1s, got %v", cfg.BaseDelay)
	}
	if cfg.MaxDelay != 30*time.Second {
		t.Errorf("Expected MaxDelay=30s, got %v", cfg.MaxDelay)
	}
	if cfg.Factor != 2.0 {
		t.Errorf("Expected Factor=2.0, got %f", cfg.Factor)
	}
	if !cfg.Jitter {
		t.Error("Expected Jitter=true")
	}
}
