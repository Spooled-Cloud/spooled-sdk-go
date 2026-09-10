package resources

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/spooled-cloud/spooled-sdk-go/internal/httpx"
)

func TestCustom_AcceptsEmpty200(t *testing.T) {
	var gotMethod, gotPath, gotToken string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotToken = r.Header.Get("X-Webhook-Token")
		_, _ = io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	res := NewIngestResource(httpx.NewTransport(httpx.Config{BaseURL: server.URL, APIKey: "sp_test_key"}))
	req := &CustomWebhookRequest{
		QueueName: "events",
		Payload:   map[string]any{"ok": true},
	}

	if err := res.Custom(context.Background(), "org_1", req); err != nil {
		t.Fatalf("Custom: %v", err)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if gotPath != "/api/v1/webhooks/org_1/custom" {
		t.Errorf("path = %q, want /api/v1/webhooks/org_1/custom", gotPath)
	}

	if err := res.CustomWithToken(context.Background(), "org_1", "whk_test", req); err != nil {
		t.Fatalf("CustomWithToken: %v", err)
	}
	if gotToken != "whk_test" {
		t.Errorf("X-Webhook-Token = %q, want whk_test", gotToken)
	}
}
