package spooled

import (
	"testing"
)

func TestNewClient_WithAPIKey(t *testing.T) {
	client, err := NewClient(
		WithAPIKey("sp_test_123456789012345678901234567890"),
	)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	defer client.Close()

	cfg := client.GetConfig()
	if cfg.APIKey != "sp_test_123456789012345678901234567890" {
		t.Errorf("Expected API key to be set")
	}
}

func TestNewClient_WithAccessToken(t *testing.T) {
	client, err := NewClient(
		WithAccessToken("jwt-token-here"),
	)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	defer client.Close()

	cfg := client.GetConfig()
	if cfg.AccessToken != "jwt-token-here" {
		t.Errorf("Expected access token to be set")
	}
}

func TestNewClient_NoAuth(t *testing.T) {
	_, err := NewClient()
	if err == nil {
		t.Error("Expected error when no authentication provided")
	}
	if err != ErrNoAuth {
		t.Errorf("Expected ErrNoAuth, got %v", err)
	}
}

func TestNewClient_InvalidAPIKey(t *testing.T) {
	_, err := NewClient(
		WithAPIKey("invalid"),
	)
	if err == nil {
		t.Error("Expected error for invalid API key")
	}
	if err != ErrInvalidAPIKey {
		t.Errorf("Expected ErrInvalidAPIKey, got %v", err)
	}
}

func TestNewClient_AllOptions(t *testing.T) {
	logger := LoggerFunc(func(msg string, keysAndValues ...any) {})

	client, err := NewClient(
		WithAPIKey("sp_live_123456789012345678901234567890"),
		WithAdminKey("admin-key"),
		WithBaseURL("https://custom.api.example.com"),
		WithGRPCAddress("grpc.example.com:443"),
		WithRetry(RetryConfig{MaxRetries: 5}),
		WithCircuitBreaker(CircuitBreakerConfig{Enabled: true}),
		WithHeaders(map[string]string{"X-Custom": "value"}),
		WithLogger(logger),
		WithAutoRefreshToken(false),
	)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	defer client.Close()

	cfg := client.GetConfig()
	if cfg.BaseURL != "https://custom.api.example.com" {
		t.Errorf("BaseURL = %q, want %q", cfg.BaseURL, "https://custom.api.example.com")
	}
	if cfg.AdminKey != "admin-key" {
		t.Error("AdminKey not set")
	}
	if cfg.GRPCAddress != "grpc.example.com:443" {
		t.Errorf("GRPCAddress = %q, want %q", cfg.GRPCAddress, "grpc.example.com:443")
	}
	if cfg.Retry.MaxRetries != 5 {
		t.Errorf("Retry.MaxRetries = %d, want 5", cfg.Retry.MaxRetries)
	}
	if cfg.AutoRefreshToken != false {
		t.Error("AutoRefreshToken should be false")
	}
}

func TestNewClient_WSURLDerived(t *testing.T) {
	client, err := NewClient(
		WithAPIKey("sp_test_123456789012345678901234567890"),
		WithBaseURL("https://custom.example.com"),
	)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	defer client.Close()

	cfg := client.GetConfig()
	if cfg.WSURL != "wss://custom.example.com" {
		t.Errorf("WSURL = %q, want %q", cfg.WSURL, "wss://custom.example.com")
	}
}

func TestNewClient_ResourcesInitialized(t *testing.T) {
	client, err := NewClient(
		WithAPIKey("sp_test_123456789012345678901234567890"),
	)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	defer client.Close()

	// Verify all resources are initialized
	if client.Jobs() == nil {
		t.Error("Jobs resource not initialized")
	}
	if client.Queues() == nil {
		t.Error("Queues resource not initialized")
	}
	if client.Workers() == nil {
		t.Error("Workers resource not initialized")
	}
	if client.Schedules() == nil {
		t.Error("Schedules resource not initialized")
	}
	if client.Workflows() == nil {
		t.Error("Workflows resource not initialized")
	}
	if client.Webhooks() == nil {
		t.Error("Webhooks resource not initialized")
	}
	if client.Organizations() == nil {
		t.Error("Organizations resource not initialized")
	}
	if client.APIKeys() == nil {
		t.Error("APIKeys resource not initialized")
	}
	if client.Billing() == nil {
		t.Error("Billing resource not initialized")
	}
	if client.Dashboard() == nil {
		t.Error("Dashboard resource not initialized")
	}
	if client.Health() == nil {
		t.Error("Health resource not initialized")
	}
	if client.Metrics() == nil {
		t.Error("Metrics resource not initialized")
	}
	if client.Auth() == nil {
		t.Error("Auth resource not initialized")
	}
	if client.Admin() == nil {
		t.Error("Admin resource not initialized")
	}
	if client.Ingest() == nil {
		t.Error("Ingest resource not initialized")
	}
}

func TestValidateAPIKey(t *testing.T) {
	tests := []struct {
		key     string
		wantErr bool
	}{
		// New prefix keys (sp_)
		{"sp_live_123456789012345678901234567890", false},
		{"sp_test_123456789012345678901234567890", false},
		// Legacy prefix keys (sk_) - use clearly fake values to avoid GitHub detection
		{"sk_live_FAKE_TEST_KEY_12345678901234", false},
		{"sk_test_FAKE_TEST_KEY_12345678901234", false},
		// Invalid keys
		{"sp_live_short", true}, // too short
		{"sp_test_", true},      // too short
		{"invalid_key", true},   // wrong prefix
		{"", true},              // empty
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			err := ValidateAPIKey(tt.key)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateAPIKey(%q) error = %v, wantErr %v", tt.key, err, tt.wantErr)
			}
		})
	}
}
