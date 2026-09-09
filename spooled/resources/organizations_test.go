package resources

import (
	"context"
	"encoding/json"
	"io"
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

func TestClearWebhookToken_PostsConfirmToClearRoute(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		data, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(data, &gotBody); err != nil {
			t.Errorf("invalid body %s: %v", data, err)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	res := NewOrganizationsResource(httpx.NewTransport(httpx.Config{BaseURL: server.URL, APIKey: "sp_test_key"}))
	if err := res.ClearWebhookToken(context.Background(), "org_1"); err != nil {
		t.Fatalf("ClearWebhookToken: %v", err)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if gotPath != "/api/v1/organizations/webhook-token/clear" {
		t.Errorf("path = %q, want /api/v1/organizations/webhook-token/clear", gotPath)
	}
	if gotBody["confirm"] != true {
		t.Errorf("body = %v, want confirm:true", gotBody)
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
