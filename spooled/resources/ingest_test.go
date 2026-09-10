package resources

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/spooled-cloud/spooled-sdk-go/internal/httpx"
)

func TestCustom_MapsWebhookResponse(t *testing.T) {
	var gotPath, gotToken string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotToken = r.Header.Get("X-Webhook-Token")
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"job_id":"job_1","queue_name":"events","status":"pending"}`))
	}))
	defer server.Close()

	res := NewIngestResource(httpx.NewTransport(httpx.Config{BaseURL: server.URL, APIKey: "sp_test_key"}))
	req := &CustomWebhookRequest{
		QueueName: "events",
		Payload:   map[string]any{"ok": true},
	}

	got, err := res.Custom(context.Background(), "org_1", req)
	if err != nil {
		t.Fatalf("Custom: %v", err)
	}
	if gotPath != "/api/v1/webhooks/org_1/custom" {
		t.Errorf("path = %q, want /api/v1/webhooks/org_1/custom", gotPath)
	}
	if got == nil || got.JobID != "job_1" || got.QueueName != "events" || got.Status != "pending" {
		t.Errorf("Custom = %+v, want job_1/events/pending", got)
	}

	got, err = res.CustomWithToken(context.Background(), "org_1", "whk_test", req)
	if err != nil {
		t.Fatalf("CustomWithToken: %v", err)
	}
	if gotToken != "whk_test" {
		t.Errorf("X-Webhook-Token = %q, want whk_test", gotToken)
	}
	if got == nil || got.JobID != "job_1" {
		t.Errorf("CustomWithToken JobID = %v, want job_1", got)
	}
}

func TestCustom_AcceptsEmpty200(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	res := NewIngestResource(httpx.NewTransport(httpx.Config{BaseURL: server.URL, APIKey: "sp_test_key"}))
	req := &CustomWebhookRequest{QueueName: "events", Payload: map[string]any{"ok": true}}

	got, err := res.Custom(context.Background(), "org_1", req)
	if err != nil {
		t.Fatalf("Custom: %v", err)
	}
	if got == nil || got.JobID != "" {
		t.Errorf("empty 200 JobID = %v, want empty", got)
	}

	got, err = res.CustomWithToken(context.Background(), "org_1", "whk_test", req)
	if err != nil {
		t.Fatalf("CustomWithToken: %v", err)
	}
	if got == nil || got.JobID != "" {
		t.Errorf("empty 200 token JobID = %v, want empty", got)
	}
}
