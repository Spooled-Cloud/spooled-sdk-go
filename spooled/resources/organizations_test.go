package resources

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/spooled-cloud/spooled-sdk-go/internal/httpx"
)

func TestGetWebhookToken_UsesAuthScopedRoute(t *testing.T) {
	var gotMethod, gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"webhook_token":"whk_abc123","webhook_url":"https://api.spooled.cloud/api/v1/webhooks/org_1/custom"}`))
	}))
	defer server.Close()

	res := NewOrganizationsResource(httpx.NewTransport(httpx.Config{BaseURL: server.URL, APIKey: "sp_test_key"}))
	got, err := res.GetWebhookToken(context.Background(), "org_1")
	if err != nil {
		t.Fatalf("GetWebhookToken: %v", err)
	}
	if gotMethod != http.MethodGet {
		t.Errorf("method = %q, want GET", gotMethod)
	}
	if gotPath != "/api/v1/organizations/webhook-token" {
		t.Errorf("path = %q, want /api/v1/organizations/webhook-token", gotPath)
	}
	if got.WebhookToken == nil || *got.WebhookToken != "whk_abc123" {
		t.Errorf("WebhookToken = %v, want whk_abc123", got.WebhookToken)
	}
	if got.WebhookURL == nil || *got.WebhookURL != "https://api.spooled.cloud/api/v1/webhooks/org_1/custom" {
		t.Errorf("WebhookURL = %v", got.WebhookURL)
	}
}

func TestRegenerateWebhookToken_UsesAuthScopedRoute(t *testing.T) {
	var gotMethod, gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"webhook_token":"whk_new456","webhook_url":"https://api.spooled.cloud/api/v1/webhooks/org_1/custom"}`))
	}))
	defer server.Close()

	res := NewOrganizationsResource(httpx.NewTransport(httpx.Config{BaseURL: server.URL, APIKey: "sp_test_key"}))
	got, err := res.RegenerateWebhookToken(context.Background(), "org_1")
	if err != nil {
		t.Fatalf("RegenerateWebhookToken: %v", err)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if gotPath != "/api/v1/organizations/webhook-token/regenerate" {
		t.Errorf("path = %q, want /api/v1/organizations/webhook-token/regenerate", gotPath)
	}
	if got.WebhookToken == nil || *got.WebhookToken != "whk_new456" {
		t.Errorf("WebhookToken = %v, want whk_new456", got.WebhookToken)
	}
}

// TestClearWebhookToken_IsUnsupportedWithoutNetworkCall pins SDKGP/FEAT 2a: the
// API answers POST /organizations/webhook-token/clear with 422 every time (an
// unauthenticated ingestion endpoint is never allowed), so the method fails fast
// with a typed error that says so instead of a request that cannot succeed.
func TestClearWebhookToken_IsUnsupportedWithoutNetworkCall(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusUnprocessableEntity)
	}))
	defer server.Close()

	res := NewOrganizationsResource(httpx.NewTransport(httpx.Config{BaseURL: server.URL, APIKey: "sp_test_key"}))
	err := res.ClearWebhookToken(context.Background(), "org_1")
	if !errors.Is(err, ErrUnsupported) || !IsUnsupportedError(err) {
		t.Fatalf("ClearWebhookToken err = %v, want ErrUnsupported", err)
	}
	var unsupported *UnsupportedError
	if !errors.As(err, &unsupported) || unsupported.Operation != "Organizations.ClearWebhookToken" {
		t.Errorf("err = %#v, want an UnsupportedError for Organizations.ClearWebhookToken", err)
	}
	if called {
		t.Error("ClearWebhookToken made a network request")
	}
}

func TestUsage_UsesAuthScopedRoute(t *testing.T) {
	var gotMethod, gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"plan":"pro","plan_display_name":"Pro","limits":{"tier":"pro","display_name":"Pro","max_payload_size_bytes":1048576,"rate_limit_requests_per_second":100,"rate_limit_burst":200,"job_retention_days":30,"history_retention_days":30},"usage":{"jobs_today":{"current":5,"is_disabled":false},"active_jobs":{"current":1,"is_disabled":false},"queues":{"current":1,"is_disabled":false},"workers":{"current":0,"is_disabled":false},"api_keys":{"current":1,"is_disabled":false},"schedules":{"current":0,"is_disabled":false},"workflows":{"current":0,"is_disabled":false},"webhooks":{"current":0,"is_disabled":false}},"warnings":[]}`))
	}))
	defer server.Close()

	res := NewOrganizationsResource(httpx.NewTransport(httpx.Config{BaseURL: server.URL, APIKey: "sp_test_key"}))
	got, err := res.Usage(context.Background(), "org_1")
	if err != nil {
		t.Fatalf("Usage: %v", err)
	}
	if gotMethod != http.MethodGet {
		t.Errorf("method = %q, want GET", gotMethod)
	}
	if gotPath != "/api/v1/organizations/usage" {
		t.Errorf("path = %q, want /api/v1/organizations/usage", gotPath)
	}
	if got.Plan != "pro" {
		t.Errorf("Plan = %q, want pro", got.Plan)
	}
	if got.PlanDisplayName != "Pro" {
		t.Errorf("PlanDisplayName = %q, want Pro", got.PlanDisplayName)
	}
	if got.Usage.JobsToday.Current != 5 {
		t.Errorf("JobsToday.Current = %d, want 5", got.Usage.JobsToday.Current)
	}
}

func TestCheckSlug_UsesQueryNotPath(t *testing.T) {
	var gotMethod, gotPath, gotSlug string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotSlug = r.URL.Query().Get("slug")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"available":false,"valid":true,"suggestion":"acme-2"}`))
	}))
	defer server.Close()

	res := NewOrganizationsResource(httpx.NewTransport(httpx.Config{BaseURL: server.URL, APIKey: "sp_test_key"}))
	got, err := res.CheckSlug(context.Background(), "acme")
	if err != nil {
		t.Fatalf("CheckSlug: %v", err)
	}
	if gotMethod != http.MethodGet {
		t.Errorf("method = %q, want GET", gotMethod)
	}
	if gotPath != "/api/v1/organizations/check-slug" {
		t.Errorf("path = %q, want /api/v1/organizations/check-slug", gotPath)
	}
	if gotSlug != "acme" {
		t.Errorf("slug query = %q, want acme", gotSlug)
	}
	if got.Available {
		t.Errorf("Available = true, want false")
	}
	if !got.Valid {
		t.Errorf("Valid = false, want true")
	}
	if got.Suggestion == nil || *got.Suggestion != "acme-2" {
		t.Errorf("Suggestion = %v, want acme-2", got.Suggestion)
	}
}
