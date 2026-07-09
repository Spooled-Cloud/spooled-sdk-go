package spooled

import (
	"net/http"
	"strings"
	"testing"
)

// TestResolveConfig_TrimsCredentials pins the contract that resolveConfig
// strips leading/trailing whitespace (including trailing newlines and stray
// spaces) from every credential field. Without trimming, Go's net/http
// package rejects `Authorization: Bearer <key>\n` with an opaque error
// that users routinely mis-diagnose as an auth failure.
func TestResolveConfig_TrimsCredentials(t *testing.T) {
	const validKey = "sp_test_123456789012345678901234567890"

	tests := []struct {
		name  string
		raw   string
		want  string
		field string
	}{
		{"api_key_trailing_newline", validKey + "\n", validKey, "APIKey"},
		{"api_key_leading_and_trailing_space", "  " + validKey + "  ", validKey, "APIKey"},
		{"api_key_windows_newline", validKey + "\r\n", validKey, "APIKey"},
		{"access_token_newline", "jwt.header.body\n", "jwt.header.body", "AccessToken"},
		{"refresh_token_space", "  refresh-token  ", "refresh-token", "RefreshToken"},
		{"admin_key_newline", "sk_admin_secret_value\n", "sk_admin_secret_value", "AdminKey"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var opt Option
			switch tc.field {
			case "APIKey":
				opt = WithAPIKey(tc.raw)
			case "AccessToken":
				opt = WithAccessToken(tc.raw)
			case "RefreshToken":
				opt = WithRefreshToken(tc.raw)
			case "AdminKey":
				opt = WithAdminKey(tc.raw)
			default:
				t.Fatalf("unknown field %q", tc.field)
			}

			cfg := resolveConfig(opt)

			var got string
			switch tc.field {
			case "APIKey":
				got = cfg.APIKey
			case "AccessToken":
				got = cfg.AccessToken
			case "RefreshToken":
				got = cfg.RefreshToken
			case "AdminKey":
				got = cfg.AdminKey
			}
			if got != tc.want {
				t.Errorf("%s: got %q, want %q", tc.field, got, tc.want)
			}
		})
	}
}

// TestValidateAPIKey_TrimsBeforeChecking asserts that ValidateAPIKey normalizes
// whitespace before running its prefix/length rules, matching resolveConfig.
func TestValidateAPIKey_TrimsBeforeChecking(t *testing.T) {
	const validKey = "sp_test_123456789012345678901234567890"

	if err := ValidateAPIKey(validKey + "\n"); err != nil {
		t.Errorf("expected trailing newline to be tolerated, got %v", err)
	}
	if err := ValidateAPIKey("  " + validKey + "  "); err != nil {
		t.Errorf("expected surrounding spaces to be tolerated, got %v", err)
	}
	if err := ValidateAPIKey("   "); err == nil {
		t.Errorf("expected whitespace-only key to be rejected")
	}
}

// TestConfigAPIKey_YieldsValidBearerHeader is a regression check for the
// production bug where a key read via `os.Getenv` (which returned a trailing
// '\n' from a `.env` line) blew up net/http with `invalid header value`.
// After trimming, `Authorization: Bearer <key>` must contain no CR/LF and
// must be accepted by `http.Request.Header.Set`.
func TestConfigAPIKey_YieldsValidBearerHeader(t *testing.T) {
	const validKey = "sp_test_abcdefghijklmnopqrstuvwxyz1234"

	client, err := NewClient(WithAPIKey(validKey + "\n"))
	if err != nil {
		t.Fatalf("NewClient failed after trim: %v", err)
	}
	defer client.Close()

	cfg := client.GetConfig()
	if strings.ContainsAny(cfg.APIKey, "\r\n") {
		t.Fatalf("resolved APIKey still contains CR/LF: %q", cfg.APIKey)
	}

	// Actually attempt to construct the header net/http will reject if it
	// contains any control character. This mirrors the transport code path.
	req, err := http.NewRequest(http.MethodGet, "https://api.spooled.cloud/api/v1/health", nil)
	if err != nil {
		t.Fatalf("http.NewRequest failed: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)

	auth := req.Header.Get("Authorization")
	if want := "Bearer " + validKey; auth != want {
		t.Errorf("Authorization: got %q, want %q", auth, want)
	}
}
