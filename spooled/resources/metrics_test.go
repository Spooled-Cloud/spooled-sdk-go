package resources

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/spooled-cloud/spooled-sdk-go/internal/httpx"
)

func TestGet_ReadsPrometheusTextFromSlashMetrics(t *testing.T) {
	var gotMethod, gotPath string
	const body = "# HELP jobs_total Total jobs\njobs_total 3\n"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()

	res := NewMetricsResource(httpx.NewTransport(httpx.Config{BaseURL: server.URL, APIKey: "sp_test_key"}))
	got, err := res.Get(context.Background())
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if gotMethod != http.MethodGet {
		t.Errorf("method = %q, want GET", gotMethod)
	}
	if gotPath != "/metrics" {
		t.Errorf("path = %q, want /metrics", gotPath)
	}
	if got != body {
		t.Errorf("body = %q, want prometheus text", got)
	}
}
