package resources

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/spooled-cloud/spooled-sdk-go/internal/httpx"
)

func TestCheckEmail_GetsCheckEmailNotEmailCheck(t *testing.T) {
	var gotMethod, gotPath, gotEmail string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotEmail = r.URL.Query().Get("email")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"available":true,"exists":false,"signup_enabled":true}`))
	}))
	defer server.Close()

	res := NewAuthResource(httpx.NewTransport(httpx.Config{BaseURL: server.URL, APIKey: "sp_test_key"}))
	got, err := res.CheckEmail(context.Background(), "new@example.com")
	if err != nil {
		t.Fatalf("CheckEmail: %v", err)
	}
	if gotMethod != http.MethodGet {
		t.Errorf("method = %q, want GET", gotMethod)
	}
	if gotPath != "/api/v1/auth/check-email" {
		t.Errorf("path = %q, want /api/v1/auth/check-email", gotPath)
	}
	if gotEmail != "new@example.com" {
		t.Errorf("email = %q, want new@example.com", gotEmail)
	}
	if got.Exists {
		t.Errorf("Exists = true, want false")
	}
	if !got.Available {
		t.Errorf("Available = false, want true")
	}
	if !got.SignupEnabled {
		t.Errorf("SignupEnabled = false, want true")
	}
}

func TestStartEmailLogin_MapsMessageAndEmailSentTo(t *testing.T) {
	var gotMethod, gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"message":"Login code sent to your email","email_sent_to":"n***@example.com"}`))
	}))
	defer server.Close()

	res := NewAuthResource(httpx.NewTransport(httpx.Config{BaseURL: server.URL, APIKey: "sp_test_key"}))
	got, err := res.StartEmailLogin(context.Background(), &StartEmailLoginRequest{Email: "new@example.com"})
	if err != nil {
		t.Fatalf("StartEmailLogin: %v", err)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if gotPath != "/api/v1/auth/email/start" {
		t.Errorf("path = %q, want /api/v1/auth/email/start", gotPath)
	}
	if got.Message != "Login code sent to your email" {
		t.Errorf("Message = %q", got.Message)
	}
	if got.EmailSentTo != "n***@example.com" {
		t.Errorf("EmailSentTo = %q, want n***@example.com", got.EmailSentTo)
	}
}

func TestValidate_MapsClaimsNotTopLevelIDs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/auth/validate" {
			t.Errorf("path = %q, want /api/v1/auth/validate", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"valid":true,"claims":{"org_id":"org_1","api_key_id":"key_1","queues":["emails"],"exp":1700003600,"iat":1700000000}}`))
	}))
	defer server.Close()

	res := NewAuthResource(httpx.NewTransport(httpx.Config{BaseURL: server.URL, APIKey: "sp_test_key"}))
	got, err := res.Validate(context.Background(), &ValidateRequest{Token: "jwt"})
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if !got.Valid {
		t.Errorf("Valid = false, want true")
	}
	if got.OrganizationID == nil || *got.OrganizationID != "org_1" {
		t.Errorf("OrganizationID = %v, want org_1", got.OrganizationID)
	}
	if got.APIKeyID == nil || *got.APIKeyID != "key_1" {
		t.Errorf("APIKeyID = %v, want key_1", got.APIKeyID)
	}
	if len(got.Queues) != 1 || got.Queues[0] != "emails" {
		t.Errorf("Queues = %v, want [emails]", got.Queues)
	}
	if got.ExpiresAt == nil || got.ExpiresAt.Unix() != 1700003600 {
		t.Errorf("ExpiresAt = %v, want unix 1700003600", got.ExpiresAt)
	}
}
