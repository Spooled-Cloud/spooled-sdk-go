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

func TestUpdateConfig_PutsQueuesNameConfig(t *testing.T) {
	var gotMethod, gotPath string
	enabled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		body, _ := io.ReadAll(r.Body)
		var got map[string]any
		if err := json.Unmarshal(body, &got); err != nil {
			t.Errorf("request body: %v", err)
		}
		if got["enabled"] != false {
			t.Errorf("enabled = %v, want false", got["enabled"])
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"qc_1","organization_id":"org_1","queue_name":"mail","max_retries":3,"default_timeout":300,"enabled":false,"settings":{},"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}`))
	}))
	defer server.Close()

	res := NewQueuesResource(httpx.NewTransport(httpx.Config{BaseURL: server.URL, APIKey: "sp_test_key"}))
	got, err := res.UpdateConfig(context.Background(), "mail", &UpdateQueueConfigRequest{Enabled: &enabled})
	if err != nil {
		t.Fatalf("UpdateConfig: %v", err)
	}
	if gotMethod != http.MethodPut {
		t.Errorf("method = %q, want PUT", gotMethod)
	}
	if gotPath != "/api/v1/queues/mail/config" {
		t.Errorf("path = %q, want /api/v1/queues/mail/config", gotPath)
	}
	if got.QueueName != "mail" {
		t.Errorf("QueueName = %q, want mail", got.QueueName)
	}
	if got.Enabled {
		t.Errorf("Enabled = true, want false")
	}
}
