package resources

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/spooled-cloud/spooled-sdk-go/internal/httpx"
)

func strPtr(s string) *string { return &s }

func intPtr(i int) *int { return &i }

func TestClaimedJob_UnmarshalLeaseID(t *testing.T) {
	tests := []struct {
		name        string
		body        string
		wantLeaseID *string
	}{
		{
			name: "lease_id present",
			body: `{"id":"job-1","queue_name":"emails","payload":{"to":"a@b.c"},` +
				`"retry_count":0,"max_retries":3,"timeout_seconds":30,` +
				`"lease_expires_at":"2026-07-11T00:00:30Z","lease_id":"lease-abc123"}`,
			wantLeaseID: strPtr("lease-abc123"),
		},
		{
			name: "lease_id absent (legacy backend)",
			body: `{"id":"job-2","queue_name":"emails","payload":{},` +
				`"retry_count":0,"max_retries":3,"timeout_seconds":30}`,
			wantLeaseID: nil,
		},
		{
			name: "lease_id null",
			body: `{"id":"job-3","queue_name":"emails","payload":{},` +
				`"retry_count":0,"max_retries":3,"timeout_seconds":30,"lease_id":null}`,
			wantLeaseID: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var job ClaimedJob
			if err := json.Unmarshal([]byte(tt.body), &job); err != nil {
				t.Fatalf("Unmarshal failed: %v", err)
			}
			switch {
			case tt.wantLeaseID == nil && job.LeaseID != nil:
				t.Errorf("LeaseID = %q, want nil", *job.LeaseID)
			case tt.wantLeaseID != nil && job.LeaseID == nil:
				t.Errorf("LeaseID = nil, want %q", *tt.wantLeaseID)
			case tt.wantLeaseID != nil && *job.LeaseID != *tt.wantLeaseID:
				t.Errorf("LeaseID = %q, want %q", *job.LeaseID, *tt.wantLeaseID)
			}
		})
	}
}

func TestJobRequests_MarshalLeaseID(t *testing.T) {
	tests := []struct {
		name        string
		req         any
		wantLeaseID string // "" means the lease_id key must be omitted
	}{
		{
			name: "complete with lease_id",
			req: &CompleteJobRequest{
				WorkerID: "w-1",
				Result:   map[string]any{"ok": true},
				LeaseID:  strPtr("lease-abc123"),
			},
			wantLeaseID: "lease-abc123",
		},
		{
			name:        "complete without lease_id (legacy)",
			req:         &CompleteJobRequest{WorkerID: "w-1"},
			wantLeaseID: "",
		},
		{
			name: "fail with lease_id",
			req: &FailJobRequest{
				WorkerID: "w-1",
				Error:    "boom",
				LeaseID:  strPtr("lease-abc123"),
			},
			wantLeaseID: "lease-abc123",
		},
		{
			name:        "fail without lease_id (legacy)",
			req:         &FailJobRequest{WorkerID: "w-1", Error: "boom"},
			wantLeaseID: "",
		},
		{
			name: "heartbeat with lease_id",
			req: &HeartbeatRequest{
				WorkerID:         "w-1",
				LeaseDurationSec: intPtr(30),
				LeaseID:          strPtr("lease-abc123"),
			},
			wantLeaseID: "lease-abc123",
		},
		{
			name:        "heartbeat without lease_id (legacy)",
			req:         &HeartbeatRequest{WorkerID: "w-1"},
			wantLeaseID: "",
		},
		{
			name: "renew lease with lease_id",
			req: &RenewLeaseRequest{
				WorkerID:         "w-1",
				LeaseDurationSec: 30,
				LeaseID:          strPtr("lease-abc123"),
			},
			wantLeaseID: "lease-abc123",
		},
		{
			name:        "renew lease without lease_id (legacy)",
			req:         &RenewLeaseRequest{WorkerID: "w-1", LeaseDurationSec: 30},
			wantLeaseID: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.req)
			if err != nil {
				t.Fatalf("Marshal failed: %v", err)
			}

			var decoded map[string]any
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Fatalf("re-Unmarshal failed: %v", err)
			}

			got, present := decoded["lease_id"]
			if tt.wantLeaseID == "" {
				if present {
					t.Errorf("lease_id should be omitted, got %v in %s", got, data)
				}
				return
			}
			if !present {
				t.Fatalf("lease_id missing from %s", data)
			}
			if got != tt.wantLeaseID {
				t.Errorf("lease_id = %v, want %q", got, tt.wantLeaseID)
			}
			// The wire key must be snake_case lease_id, never leaseId.
			if strings.Contains(string(data), "leaseId") {
				t.Errorf("payload contains camelCase leaseId: %s", data)
			}
		})
	}
}

func TestFailWithResponse_DecodesRetryDisposition(t *testing.T) {
	tests := []struct {
		name          string
		body          string
		wantWillRetry *bool
	}{
		{name: "present true", body: `{"success":true,"will_retry":true}`, wantWillRetry: boolPtr(true)},
		{name: "present false", body: `{"success":true,"will_retry":false}`, wantWillRetry: boolPtr(false)},
		{name: "omitted", body: `{"success":true}`, wantWillRetry: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tt.body))
			}))
			defer server.Close()

			jobs := NewJobsResource(httpx.NewTransport(httpx.Config{BaseURL: server.URL, APIKey: "sp_test_key"}))
			response, err := jobs.FailWithResponse(context.Background(), "job-1", &FailJobRequest{
				WorkerID: "worker-1",
				Error:    "boom",
			})
			if err != nil {
				t.Fatalf("FailWithResponse failed: %v", err)
			}
			switch {
			case tt.wantWillRetry == nil && response.WillRetry != nil:
				t.Fatalf("WillRetry = %v, want nil", *response.WillRetry)
			case tt.wantWillRetry != nil && response.WillRetry == nil:
				t.Fatalf("WillRetry = nil, want %v", *tt.wantWillRetry)
			case tt.wantWillRetry != nil && *response.WillRetry != *tt.wantWillRetry:
				t.Fatalf("WillRetry = %v, want %v", *response.WillRetry, *tt.wantWillRetry)
			}
		})
	}
}

func boolPtr(value bool) *bool { return &value }

func TestRenewLease_ForwardsLeaseIDToHeartbeat(t *testing.T) {
	// RenewLease is implemented over the heartbeat endpoint; the fencing token
	// must survive the internal RenewLeaseRequest -> HeartbeatRequest mapping.
	var gotPath string
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		data, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(data, &gotBody); err != nil {
			t.Errorf("invalid request body %s: %v", data, err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true}`))
	}))
	defer server.Close()

	jobs := NewJobsResource(httpx.NewTransport(httpx.Config{
		BaseURL: server.URL,
		APIKey:  "sp_test_key",
	}))

	_, err := jobs.RenewLease(context.Background(), "job-1", &RenewLeaseRequest{
		WorkerID:         "w-1",
		LeaseDurationSec: 30,
		LeaseID:          strPtr("lease-abc123"),
	})
	if err != nil {
		t.Fatalf("RenewLease failed: %v", err)
	}

	if gotPath != "/api/v1/jobs/job-1/heartbeat" {
		t.Errorf("path = %q, want /api/v1/jobs/job-1/heartbeat", gotPath)
	}
	if got := gotBody["lease_id"]; got != "lease-abc123" {
		t.Errorf("lease_id = %v, want %q", got, "lease-abc123")
	}
}
