package resources

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/spooled-cloud/spooled-sdk-go/internal/httpx"
)

func TestUpdateOrganization_PatchesNotPuts(t *testing.T) {
	var gotMethod, gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"org_1","name":"Acme","slug":"acme","plan_tier":"enterprise"}`))
	}))
	defer server.Close()

	res := NewAdminResource(httpx.NewTransport(httpx.Config{BaseURL: server.URL, AdminKey: "adminkey"}))
	tier := PlanTierEnterprise
	got, err := res.UpdateOrganization(context.Background(), "org_1", &AdminUpdateOrganizationRequest{PlanTier: &tier})
	if err != nil {
		t.Fatalf("UpdateOrganization: %v", err)
	}
	if gotMethod != http.MethodPatch {
		t.Errorf("method = %q, want PATCH", gotMethod)
	}
	if gotPath != "/api/v1/admin/organizations/org_1" {
		t.Errorf("path = %q, want /api/v1/admin/organizations/org_1", gotPath)
	}
	if got == nil || got.ID != "org_1" {
		t.Errorf("org = %+v, want id org_1", got)
	}
}

func TestGetStats_ReadsNestedPlatformPayload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/admin/stats" {
			t.Errorf("request = %s %s, want GET /api/v1/admin/stats", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"organizations": {
				"total": 4,
				"by_plan": [{"plan": "free", "count": 3}],
				"created_today": 1,
				"created_this_week": 2
			},
			"jobs": {
				"total_active": 5,
				"pending": 2,
				"processing": 1,
				"completed_24h": 10,
				"failed_24h": 0
			},
			"workers": {"total": 3, "healthy": 2, "degraded": 1},
			"system": {"api_version": "0.1.111", "uptime_seconds": 9}
		}`))
	}))
	defer server.Close()

	res := NewAdminResource(httpx.NewTransport(httpx.Config{BaseURL: server.URL, AdminKey: "adminkey"}))
	got, err := res.GetStats(context.Background())
	if err != nil {
		t.Fatalf("GetStats: %v", err)
	}
	if got.Organizations.Total != 4 {
		t.Errorf("organizations.total = %d, want 4", got.Organizations.Total)
	}
	if got.Jobs.Pending != 2 {
		t.Errorf("jobs.pending = %d, want 2", got.Jobs.Pending)
	}
	if got.Workers.Healthy != 2 {
		t.Errorf("workers.healthy = %d, want 2", got.Workers.Healthy)
	}
	if got.System.APIVersion != "0.1.111" {
		t.Errorf("system.api_version = %q, want 0.1.111", got.System.APIVersion)
	}
}

func TestDeleteOrganization_SendsHardDeleteQuery(t *testing.T) {
	var gotMethod, gotPath, gotHardDelete string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotHardDelete = r.URL.Query().Get("hard_delete")
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	res := NewAdminResource(httpx.NewTransport(httpx.Config{BaseURL: server.URL, AdminKey: "adminkey"}))
	if err := res.DeleteOrganization(context.Background(), "org_1", true); err != nil {
		t.Fatalf("DeleteOrganization: %v", err)
	}
	if gotMethod != http.MethodDelete {
		t.Errorf("method = %q, want DELETE", gotMethod)
	}
	if gotPath != "/api/v1/admin/organizations/org_1" {
		t.Errorf("path = %q, want /api/v1/admin/organizations/org_1", gotPath)
	}
	if gotHardDelete != "true" {
		t.Errorf("hard_delete = %q, want true", gotHardDelete)
	}
}
