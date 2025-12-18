package httpx

import (
	"math"
	"math/rand"
	"time"
)

// RetryPolicy implements exponential backoff with optional jitter.
type RetryPolicy struct {
	MaxRetries int
	BaseDelay  time.Duration
	MaxDelay   time.Duration
	Factor     float64
	Jitter     bool
	rng        *rand.Rand
}

// NewRetryPolicy creates a new retry policy.
func NewRetryPolicy(cfg RetryConfig) *RetryPolicy {
	// Note: MaxRetries == 0 is a valid value (means no retries), so we use -1 as sentinel
	// for default. However, since we can't distinguish 0 from unset in Go, we don't
	// default MaxRetries here. Caller must explicitly set it.
	if cfg.BaseDelay == 0 {
		cfg.BaseDelay = 1 * time.Second
	}
	if cfg.MaxDelay == 0 {
		cfg.MaxDelay = 30 * time.Second
	}
	if cfg.Factor == 0 {
		cfg.Factor = 2.0
	}

	return &RetryPolicy{
		MaxRetries: cfg.MaxRetries,
		BaseDelay:  cfg.BaseDelay,
		MaxDelay:   cfg.MaxDelay,
		Factor:     cfg.Factor,
		Jitter:     cfg.Jitter,
		rng:        rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// Delay calculates the delay for a given retry attempt (0-indexed).
func (p *RetryPolicy) Delay(attempt int) time.Duration {
	// Calculate exponential delay
	delay := float64(p.BaseDelay) * math.Pow(p.Factor, float64(attempt))

	// Cap at max delay
	if delay > float64(p.MaxDelay) {
		delay = float64(p.MaxDelay)
	}

	// Add jitter if enabled (±25% of delay)
	if p.Jitter {
		jitterFactor := 0.5 + p.rng.Float64() // 0.5 to 1.5
		delay = delay * jitterFactor
	}

	return time.Duration(delay)
}

// DelayWithJitter calculates delay with deterministic jitter for testing.
func (p *RetryPolicy) DelayWithJitter(attempt int, jitterFactor float64) time.Duration {
	// Calculate exponential delay
	delay := float64(p.BaseDelay) * math.Pow(p.Factor, float64(attempt))

	// Cap at max delay
	if delay > float64(p.MaxDelay) {
		delay = float64(p.MaxDelay)
	}

	// Apply jitter factor (expected to be 0.5 to 1.5)
	delay = delay * jitterFactor

	return time.Duration(delay)
}

// ShouldRetry returns true if we haven't exhausted retry attempts.
func (p *RetryPolicy) ShouldRetry(attempt int) bool {
	return attempt < p.MaxRetries
}
