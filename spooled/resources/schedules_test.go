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

func TestCreate_MapsPartialCreateResponseOntoSchedule(t *testing.T) {
	tz := "America/New_York"
	priority := 5
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/schedules" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		body, _ := io.ReadAll(r.Body)
		var got map[string]any
		if err := json.Unmarshal(body, &got); err != nil {
			t.Errorf("request body: %v", err)
		}
		if got["queue_name"] != "mail" {
			t.Errorf("queue_name = %v, want mail", got["queue_name"])
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"sch_1","name":"Nightly","cron_expression":"0 0 * * *","next_run_at":"2026-01-02T00:00:00Z"}`))
	}))
	defer server.Close()

	res := NewSchedulesResource(httpx.NewTransport(httpx.Config{BaseURL: server.URL, APIKey: "sp_test_key"}))
	got, err := res.Create(context.Background(), &CreateScheduleRequest{
		Name:            "Nightly",
		CronExpression:  "0 0 * * *",
		Timezone:        &tz,
		QueueName:       "mail",
		PayloadTemplate: map[string]any{"job_type": "digest"},
		Priority:        &priority,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if got.ID != "sch_1" {
		t.Errorf("ID = %q, want sch_1", got.ID)
	}
	if !got.IsActive {
		t.Errorf("IsActive = false, want true (create inserts is_active TRUE)")
	}
	if got.QueueName != "mail" {
		t.Errorf("QueueName = %q, want mail", got.QueueName)
	}
	if got.Timezone != tz {
		t.Errorf("Timezone = %q, want %s", got.Timezone, tz)
	}
	payload, ok := got.PayloadTemplate.(map[string]any)
	if !ok || payload["job_type"] != "digest" {
		t.Errorf("PayloadTemplate = %v", got.PayloadTemplate)
	}
	if got.Priority != 5 {
		t.Errorf("Priority = %d, want 5", got.Priority)
	}
	if got.NextRunAt == nil {
		t.Fatal("NextRunAt = nil")
	}
}

func TestSchedule_UnmarshalNonObjectJSON(t *testing.T) {
	body := `{
		"id":"sch_1","organization_id":"org_1","name":"Nightly",
		"cron_expression":"0 0 * * *","timezone":"UTC","queue_name":"mail",
		"payload_template":["a"],"priority":0,"max_retries":3,"timeout_seconds":300,
		"is_active":true,"run_count":0,"tags":["urgent"],"metadata":true,
		"created_at":"2024-01-01T00:00:00Z","updated_at":"2024-01-01T00:00:00Z"
	}`
	var got Schedule
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	payload, ok := got.PayloadTemplate.([]any)
	if !ok || len(payload) != 1 {
		t.Errorf("PayloadTemplate = %#v, want [a]", got.PayloadTemplate)
	}
	tags, ok := got.Tags.([]any)
	if !ok || len(tags) != 1 {
		t.Errorf("Tags = %#v, want [urgent]", got.Tags)
	}
	if got.Metadata != true {
		t.Errorf("Metadata = %#v, want true", got.Metadata)
	}
}
