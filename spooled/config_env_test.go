package spooled

import "testing"

const envTestKey = "sp_test_env00000000000000000000000000"

func TestNewClient_ReadsEnvironment(t *testing.T) {
	t.Setenv(EnvAPIKey, " "+envTestKey+"\n")
	t.Setenv(EnvBaseURL, "https://spooled.example.com/")
	t.Setenv(EnvGRPCAddress, "grpc.example.com:8443")

	client, err := NewClient()
	if err != nil {
		t.Fatalf("NewClient with SPOOLED_API_KEY set: %v", err)
	}
	defer client.Close()

	cfg := client.GetConfig()
	if cfg.APIKey != envTestKey {
		t.Errorf("APIKey = %q, want %q", cfg.APIKey, envTestKey)
	}
	if cfg.BaseURL != "https://spooled.example.com" {
		t.Errorf("BaseURL = %q, want https://spooled.example.com", cfg.BaseURL)
	}
	if cfg.WSURL != "wss://spooled.example.com" {
		t.Errorf("WSURL = %q, want it derived from SPOOLED_BASE_URL", cfg.WSURL)
	}
	if cfg.GRPCAddress != "grpc.example.com:8443" {
		t.Errorf("GRPCAddress = %q, want grpc.example.com:8443", cfg.GRPCAddress)
	}
}

func TestNewClient_OptionsWinOverEnvironment(t *testing.T) {
	t.Setenv(EnvAPIKey, envTestKey)
	t.Setenv(EnvBaseURL, "https://env.example.com")
	t.Setenv(EnvGRPCAddress, "grpc.env.example.com:443")

	const optionKey = "sp_live_option000000000000000000000000"
	client, err := NewClient(
		WithAPIKey(optionKey),
		WithBaseURL("http://localhost:8080"),
		WithGRPCAddress("localhost:50051"),
	)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer client.Close()

	cfg := client.GetConfig()
	if cfg.APIKey != optionKey {
		t.Errorf("APIKey = %q, want the option's key", cfg.APIKey)
	}
	if cfg.BaseURL != "http://localhost:8080" {
		t.Errorf("BaseURL = %q, want the option's URL", cfg.BaseURL)
	}
	if cfg.GRPCAddress != "localhost:50051" {
		t.Errorf("GRPCAddress = %q, want the option's address", cfg.GRPCAddress)
	}
}

func TestNewClient_AccessTokenSuppressesEnvAPIKey(t *testing.T) {
	t.Setenv(EnvAPIKey, envTestKey)

	client, err := NewClient(WithAccessToken("jwt-token"))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer client.Close()

	cfg := client.GetConfig()
	if cfg.APIKey != "" {
		t.Errorf("APIKey = %q, want empty when an access token is given", cfg.APIKey)
	}
	if cfg.AccessToken != "jwt-token" {
		t.Errorf("AccessToken = %q, want jwt-token", cfg.AccessToken)
	}
}

func TestNewClient_InvalidEnvAPIKeyIsRejected(t *testing.T) {
	t.Setenv(EnvAPIKey, "not-a-key")
	if _, err := NewClient(); err != ErrInvalidAPIKey {
		t.Fatalf("err = %v, want ErrInvalidAPIKey", err)
	}
}
