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
