// Package spooled provides the official Go SDK for Spooled Cloud.
package spooled

import (
	"fmt"
	"strings"
	"time"

	"github.com/spooled-cloud/spooled-sdk-go/internal/version"
)

// Default configuration values
const (
	DefaultBaseURL     = "https://api.spooled.cloud"
	DefaultWSURL       = "wss://api.spooled.cloud"
	DefaultGRPCAddress = "grpc.spooled.cloud:443"
	DefaultTimeout     = 30 * time.Second
	DefaultAPIVersion  = "v1"
	DefaultAPIBasePath = "/api/v1"
)

// RetryConfig configures retry behavior for failed requests.
type RetryConfig struct {
	// MaxRetries is the maximum number of retry attempts.
	MaxRetries int
	// BaseDelay is the initial delay before the first retry.
	BaseDelay time.Duration
	// MaxDelay is the maximum delay between retries.
	MaxDelay time.Duration
	// Factor is the exponential backoff multiplier.
	Factor float64
	// Jitter enables randomized jitter on retry delays.
	Jitter bool
}

// DefaultRetryConfig returns the default retry configuration.
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries: 3,
		BaseDelay:  1 * time.Second,
		MaxDelay:   30 * time.Second,
		Factor:     2.0,
		Jitter:     true,
	}
}

// CircuitBreakerConfig configures the circuit breaker.
type CircuitBreakerConfig struct {
	// Enabled determines if circuit breaker is active.
	Enabled bool
	// FailureThreshold is the number of failures before opening the circuit.
	FailureThreshold int
	// SuccessThreshold is the number of successes needed to close the circuit.
	SuccessThreshold int
	// Timeout is the duration the circuit stays open before allowing a test request.
	Timeout time.Duration
}

// DefaultCircuitBreakerConfig returns the default circuit breaker configuration.
func DefaultCircuitBreakerConfig() CircuitBreakerConfig {
	return CircuitBreakerConfig{
		Enabled:          true,
		FailureThreshold: 5,
		SuccessThreshold: 3,
		Timeout:          30 * time.Second,
	}
}

// Logger is the interface for debug logging.
type Logger interface {
	Debug(msg string, keysAndValues ...any)
}

// LoggerFunc is a function adapter for Logger.
type LoggerFunc func(msg string, keysAndValues ...any)

// Debug implements Logger.
func (f LoggerFunc) Debug(msg string, keysAndValues ...any) {
	f(msg, keysAndValues...)
}

// Config holds the SDK configuration.
type Config struct {
	// APIKey is the API key for authentication (production keys start with sk_live_, sk_test_).
	APIKey string
	// AccessToken is a JWT access token (alternative to API key).
	AccessToken string
	// RefreshToken is a JWT refresh token for automatic token renewal.
	RefreshToken string
	// AdminKey is the admin API key (for /api/v1/admin/* endpoints; uses X-Admin-Key header).
	AdminKey string

	// BaseURL is the base URL for the REST API.
	BaseURL string
	// WSURL is the WebSocket URL for realtime events.
	WSURL string
	// GRPCAddress is the gRPC server address.
	GRPCAddress string

	// Timeout is the request timeout.
	Timeout time.Duration
	// Retry is the retry configuration.
	Retry RetryConfig
	// CircuitBreaker is the circuit breaker configuration.
	CircuitBreaker CircuitBreakerConfig

	// Headers are additional headers to include in all requests.
	Headers map[string]string
	// UserAgent is the custom user agent string.
	UserAgent string
	// Logger is the debug logger.
	Logger Logger
	// AutoRefreshToken enables automatic token refresh.
	AutoRefreshToken bool
}

// Option is a functional option for configuring the client.
type Option func(*Config)

// WithAPIKey sets the API key for authentication.
func WithAPIKey(key string) Option {
	return func(c *Config) {
		c.APIKey = key
	}
}

// WithAccessToken sets the JWT access token.
func WithAccessToken(token string) Option {
	return func(c *Config) {
		c.AccessToken = token
	}
}

// WithRefreshToken sets the JWT refresh token.
func WithRefreshToken(token string) Option {
	return func(c *Config) {
		c.RefreshToken = token
	}
}

// WithAdminKey sets the admin API key.
func WithAdminKey(key string) Option {
	return func(c *Config) {
		c.AdminKey = key
	}
}

// WithBaseURL sets the base URL for the REST API.
func WithBaseURL(url string) Option {
	return func(c *Config) {
		c.BaseURL = strings.TrimSuffix(url, "/")
	}
}

// WithWSURL sets the WebSocket URL.
func WithWSURL(url string) Option {
	return func(c *Config) {
		c.WSURL = strings.TrimSuffix(url, "/")
	}
}

// WithGRPCAddress sets the gRPC server address.
func WithGRPCAddress(addr string) Option {
	return func(c *Config) {
		c.GRPCAddress = addr
	}
}

// WithTimeout sets the request timeout.
func WithTimeout(d time.Duration) Option {
	return func(c *Config) {
		c.Timeout = d
	}
}

// WithRetry sets the retry configuration.
func WithRetry(cfg RetryConfig) Option {
	return func(c *Config) {
		c.Retry = cfg
	}
}

// WithCircuitBreaker sets the circuit breaker configuration.
func WithCircuitBreaker(cfg CircuitBreakerConfig) Option {
	return func(c *Config) {
		c.CircuitBreaker = cfg
	}
}

// WithHeaders sets additional headers for all requests.
func WithHeaders(headers map[string]string) Option {
	return func(c *Config) {
		if c.Headers == nil {
			c.Headers = make(map[string]string)
		}
		for k, v := range headers {
			c.Headers[k] = v
		}
	}
}

// WithUserAgent sets a custom user agent string.
func WithUserAgent(ua string) Option {
	return func(c *Config) {
		c.UserAgent = ua
	}
}

// WithLogger sets the debug logger.
func WithLogger(l Logger) Option {
	return func(c *Config) {
		c.Logger = l
	}
}

// WithDebug enables debug logging to stdout.
func WithDebug(enabled bool) Option {
	return func(c *Config) {
		if enabled {
			c.Logger = LoggerFunc(func(msg string, keysAndValues ...any) {
				// Simple debug output with key-value pairs
				parts := []any{"[spooled-sdk]", msg}
				for i := 0; i < len(keysAndValues); i += 2 {
					if i+1 < len(keysAndValues) {
						parts = append(parts, fmt.Sprintf("%v=%v", keysAndValues[i], keysAndValues[i+1]))
					}
				}
				fmt.Println(parts...)
			})
		}
	}
}

// WithAutoRefreshToken enables or disables automatic token refresh.
func WithAutoRefreshToken(enabled bool) Option {
	return func(c *Config) {
		c.AutoRefreshToken = enabled
	}
}

// newDefaultConfig creates a new config with default values.
func newDefaultConfig() *Config {
	return &Config{
		BaseURL:          DefaultBaseURL,
		WSURL:            DefaultWSURL,
		GRPCAddress:      DefaultGRPCAddress,
		Timeout:          DefaultTimeout,
		Retry:            DefaultRetryConfig(),
		CircuitBreaker:   DefaultCircuitBreakerConfig(),
		Headers:          make(map[string]string),
		UserAgent:        version.UserAgent(),
		AutoRefreshToken: true,
	}
}

// resolveConfig applies options and resolves derived values.
func resolveConfig(opts ...Option) *Config {
	cfg := newDefaultConfig()
	for _, opt := range opts {
		opt(cfg)
	}

	// Derive WS URL from base URL if not explicitly set
	if cfg.WSURL == DefaultWSURL && cfg.BaseURL != DefaultBaseURL {
		cfg.WSURL = deriveWSURL(cfg.BaseURL)
	}

	return cfg
}

// deriveWSURL converts an HTTP URL to a WebSocket URL.
func deriveWSURL(baseURL string) string {
	if strings.HasPrefix(baseURL, "https://") {
		return "wss://" + strings.TrimPrefix(baseURL, "https://")
	}
	if strings.HasPrefix(baseURL, "http://") {
		return "ws://" + strings.TrimPrefix(baseURL, "http://")
	}
	return baseURL
}
