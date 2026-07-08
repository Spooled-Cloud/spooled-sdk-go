package spooled_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/spooled-cloud/spooled-sdk-go/internal/httpx"
	"github.com/spooled-cloud/spooled-sdk-go/internal/testutil"
	"github.com/spooled-cloud/spooled-sdk-go/spooled"
	"github.com/spooled-cloud/spooled-sdk-go/spooled/resources"
)

const testAPIKey = "sp_test_123456789012345678901234567890"

// TestPublicHelpers_MatchTransportErrors constructs the exact error values the
// transport hands back (via httpx.ParseErrorFromResponse) and asserts that the
// public spooled.Is*Error / AsSpooledError helpers classify them correctly.
//
// This is a regression test for the v1.0.13 bug where the public helpers checked
// a parallel set of types the client never returned, so they always reported false.
func TestPublicHelpers_MatchTransportErrors(t *testing.T) {
	notFound := httpx.ParseErrorFromResponse(
		http.StatusNotFound,
		[]byte(`{"code":"NOT_FOUND","message":"job not found"}`),
		http.Header{},
	)
	validation := httpx.ParseErrorFromResponse(
		http.StatusBadRequest,
		[]byte(`{"code":"VALIDATION_ERROR","message":"queue_name is required"}`),
		http.Header{},
	)

	// --- 404 ---
	if !spooled.IsNotFoundError(notFound) {
		t.Errorf("IsNotFoundError(404) = false, want true (type %T)", notFound)
	}
	if !spooled.IsSpooledError(notFound) {
		t.Error("IsSpooledError(404) = false, want true")
	}
	if spooled.IsValidationError(notFound) {
		t.Error("IsValidationError(404) = true, want false")
	}
	if apiErr, ok := spooled.AsSpooledError(notFound); !ok {
		t.Error("AsSpooledError(404) ok = false, want true")
	} else {
		if apiErr.StatusCode != http.StatusNotFound {
			t.Errorf("AsSpooledError(404).StatusCode = %d, want 404", apiErr.StatusCode)
		}
		if apiErr.Code != "NOT_FOUND" {
			t.Errorf("AsSpooledError(404).Code = %q, want NOT_FOUND", apiErr.Code)
		}
		if apiErr.Message != "job not found" {
			t.Errorf("AsSpooledError(404).Message = %q, want %q", apiErr.Message, "job not found")
		}
	}
	// The pattern documented in doc.go must also work directly.
	var nf *spooled.NotFoundError
	if !errors.As(notFound, &nf) {
		t.Error("errors.As(404, *spooled.NotFoundError) = false, want true")
	}

	// --- 400 ---
	if !spooled.IsValidationError(validation) {
		t.Errorf("IsValidationError(400) = false, want true (type %T)", validation)
	}
	if !spooled.IsSpooledError(validation) {
		t.Error("IsSpooledError(400) = false, want true")
	}
	if spooled.IsNotFoundError(validation) {
		t.Error("IsNotFoundError(400) = true, want false")
	}
	if apiErr, ok := spooled.AsSpooledError(validation); !ok {
		t.Error("AsSpooledError(400) ok = false, want true")
	} else if apiErr.StatusCode != http.StatusBadRequest || apiErr.Code != "VALIDATION_ERROR" {
		t.Errorf("AsSpooledError(400) = {%d, %q}, want {400, VALIDATION_ERROR}", apiErr.StatusCode, apiErr.Code)
	}
	var ve *spooled.ValidationError
	if !errors.As(validation, &ve) {
		t.Error("errors.As(400, *spooled.ValidationError) = false, want true")
	}
}

// TestPublicHelpers_OtherStatuses covers the remaining classifiers.
func TestPublicHelpers_OtherStatuses(t *testing.T) {
	cases := []struct {
		status int
		check  func(error) bool
		name   string
	}{
		{http.StatusUnauthorized, spooled.IsAuthenticationError, "IsAuthenticationError"},
		{http.StatusForbidden, spooled.IsAuthorizationError, "IsAuthorizationError"},
		{http.StatusConflict, spooled.IsConflictError, "IsConflictError"},
		{http.StatusTooManyRequests, spooled.IsRateLimitError, "IsRateLimitError"},
		{http.StatusRequestEntityTooLarge, spooled.IsPayloadTooLargeError, "IsPayloadTooLargeError"},
		{http.StatusBadGateway, spooled.IsServerError, "IsServerError"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := httpx.ParseErrorFromResponse(tc.status, nil, http.Header{})
			if !tc.check(err) {
				t.Errorf("%s(%d) = false, want true (type %T)", tc.name, tc.status, err)
			}
			if !spooled.IsSpooledError(err) {
				t.Errorf("IsSpooledError(%d) = false, want true", tc.status)
			}
		})
	}
}

// TestIsRetryable_NotBroken verifies the interface-based IsRetryable still works
// for retryable classes and correctly rejects non-retryable client errors.
func TestIsRetryable_NotBroken(t *testing.T) {
	retryable := []error{
		httpx.ParseErrorFromResponse(http.StatusInternalServerError, nil, http.Header{}),
		httpx.ParseErrorFromResponse(http.StatusTooManyRequests, nil, http.Header{}),
		httpx.NewNetworkError(errors.New("connection refused")),
	}
	for _, err := range retryable {
		if !spooled.IsRetryable(err) {
			t.Errorf("IsRetryable(%T) = false, want true", err)
		}
	}

	nonRetryable := []error{
		httpx.ParseErrorFromResponse(http.StatusNotFound, nil, http.Header{}),
		httpx.ParseErrorFromResponse(http.StatusBadRequest, nil, http.Header{}),
		httpx.NewCircuitBreakerOpenError(),
	}
	for _, err := range nonRetryable {
		if spooled.IsRetryable(err) {
			t.Errorf("IsRetryable(%T) = true, want false", err)
		}
	}
}

// TestClientError_NotFound_EndToEnd drives a real client through a mocked HTTP
// transport and asserts the public helpers classify the returned error.
func TestClientError_NotFound_EndToEnd(t *testing.T) {
	ms := testutil.NewMockServer(t)
	const jobID = "00000000-0000-0000-0000-000000000000"
	ms.HandleError(http.MethodGet, "/api/v1/jobs/"+jobID, http.StatusNotFound, "NOT_FOUND", "job not found")

	client, err := spooled.NewClient(
		spooled.WithAPIKey(testAPIKey),
		spooled.WithBaseURL(ms.URL),
		spooled.WithRetry(spooled.RetryConfig{MaxRetries: 0}),
	)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer client.Close()

	_, err = client.Jobs().Get(context.Background(), jobID)
	if err == nil {
		t.Fatal("expected error for missing job, got nil")
	}

	if !spooled.IsNotFoundError(err) {
		t.Errorf("IsNotFoundError(err) = false, want true (type %T)", err)
	}
	if !spooled.IsSpooledError(err) {
		t.Error("IsSpooledError(err) = false, want true")
	}
	apiErr, ok := spooled.AsSpooledError(err)
	if !ok {
		t.Fatal("AsSpooledError(err) ok = false, want true")
	}
	if apiErr.StatusCode != http.StatusNotFound {
		t.Errorf("StatusCode = %d, want 404", apiErr.StatusCode)
	}
	if apiErr.Code != "NOT_FOUND" {
		t.Errorf("Code = %q, want NOT_FOUND", apiErr.Code)
	}
	if spooled.IsRetryable(err) {
		t.Error("IsRetryable(404) = true, want false")
	}
}

// TestClientError_Validation_EndToEnd drives a real client Create call that the
// backend rejects with a 400 and asserts the public helpers classify it.
func TestClientError_Validation_EndToEnd(t *testing.T) {
	ms := testutil.NewMockServer(t)
	ms.HandleError(http.MethodPost, "/api/v1/jobs", http.StatusBadRequest, "VALIDATION_ERROR", "queue_name is required")

	client, err := spooled.NewClient(
		spooled.WithAPIKey(testAPIKey),
		spooled.WithBaseURL(ms.URL),
		spooled.WithRetry(spooled.RetryConfig{MaxRetries: 0}),
	)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer client.Close()

	_, err = client.Jobs().Create(context.Background(), &resources.CreateJobRequest{
		QueueName: "", // invalid: empty queue name
		Payload:   map[string]any{"test": true},
	})
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}

	if !spooled.IsValidationError(err) {
		t.Errorf("IsValidationError(err) = false, want true (type %T)", err)
	}
	if !spooled.IsSpooledError(err) {
		t.Error("IsSpooledError(err) = false, want true")
	}
	apiErr, ok := spooled.AsSpooledError(err)
	if !ok {
		t.Fatal("AsSpooledError(err) ok = false, want true")
	}
	if apiErr.StatusCode != http.StatusBadRequest {
		t.Errorf("StatusCode = %d, want 400", apiErr.StatusCode)
	}
	if apiErr.Code != "VALIDATION_ERROR" {
		t.Errorf("Code = %q, want VALIDATION_ERROR", apiErr.Code)
	}
}
