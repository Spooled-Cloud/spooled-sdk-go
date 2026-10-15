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

func TestWorkflow_UnmarshalProgressFromGetDetail(t *testing.T) {
	body := `{
		"id":"wf_1","name":"ETL","status":"running",
		"created_at":"2024-01-01T00:00:00Z",
		"jobs":[{"id":"job_1"},{"id":"job_2"}],
		"progress":{"total":2,"completed":1,"failed":0,"pending":1,"processing":0}
	}`
	var wf Workflow
	if err := json.Unmarshal([]byte(body), &wf); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if wf.TotalJobs != 2 {
		t.Errorf("TotalJobs = %d, want 2 (from progress.total)", wf.TotalJobs)
	}
	if wf.CompletedJobs != 1 {
		t.Errorf("CompletedJobs = %d, want 1 (from progress.completed)", wf.CompletedJobs)
	}
	if wf.FailedJobs != 0 {
		t.Errorf("FailedJobs = %d, want 0", wf.FailedJobs)
	}
}

func TestWorkflow_UnmarshalListTotalJobs(t *testing.T) {
	body := `{
		"id":"wf_1","name":"ETL","status":"running",
		"total_jobs":4,"completed_jobs":3,"failed_jobs":1,
		"created_at":"2024-01-01T00:00:00Z"
	}`
	var wf Workflow
	if err := json.Unmarshal([]byte(body), &wf); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if wf.TotalJobs != 4 {
		t.Errorf("TotalJobs = %d, want 4", wf.TotalJobs)
	}
	if wf.CompletedJobs != 3 {
		t.Errorf("CompletedJobs = %d, want 3", wf.CompletedJobs)
	}
	if wf.FailedJobs != 1 {
		t.Errorf("FailedJobs = %d, want 1", wf.FailedJobs)
	}
}

func TestGet_MapsProgressCounts(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/workflows/wf_1" {
			t.Errorf("path = %q, want /api/v1/workflows/wf_1", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"wf_1","name":"ETL","status":"running",
			"created_at":"2024-01-01T00:00:00Z",
			"jobs":[{"id":"job_1"},{"id":"job_2"}],
			"progress":{"total":2,"completed":1,"failed":0,"pending":1,"processing":0}
		}`))
	}))
	defer server.Close()

	res := NewWorkflowsResource(httpx.NewTransport(httpx.Config{BaseURL: server.URL, APIKey: "sp_test_key"}))
	wf, err := res.Get(context.Background(), "wf_1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if wf.TotalJobs != 2 {
		t.Errorf("TotalJobs = %d, want 2", wf.TotalJobs)
	}
	if wf.CompletedJobs != 1 {
		t.Errorf("CompletedJobs = %d, want 1", wf.CompletedJobs)
	}
}

func TestListJobs_ReadsWorkflowDetail(t *testing.T) {
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"wf_1",
			"name":"ETL",
			"status":"running",
			"jobs":[
				{"id":"job_1","queue":"etl","status":"completed","created_at":"2024-01-01T00:00:00Z"},
				{"id":"job_2","queue":"etl","status":"pending","created_at":"2024-01-01T00:00:00Z"}
			],
			"dependencies":[
				{"parent_job_id":"job_1","child_job_id":"job_2","dependency_type":"all"}
			]
		}`))
	}))
	defer server.Close()

	res := NewWorkflowsResource(httpx.NewTransport(httpx.Config{BaseURL: server.URL, APIKey: "sp_test_key"}))
	jobs, err := res.Jobs().ListJobs(context.Background(), "wf_1")
	if err != nil {
		t.Fatalf("ListJobs: %v", err)
	}
	if gotPath != "/api/v1/workflows/wf_1" {
		t.Errorf("path = %q, want /api/v1/workflows/wf_1", gotPath)
	}
	if len(jobs) != 2 {
		t.Fatalf("len(jobs) = %d, want 2", len(jobs))
	}
	if jobs[0].QueueName != "etl" {
		t.Errorf("jobs[0].QueueName = %q, want etl", jobs[0].QueueName)
	}
	if len(jobs[1].DependsOn) != 1 || jobs[1].DependsOn[0] != "job_1" {
		t.Errorf("jobs[1].DependsOn = %v, want [job_1]", jobs[1].DependsOn)
	}

	got, err := res.Jobs().GetJob(context.Background(), "wf_1", "job_2")
	if err != nil {
		t.Fatalf("GetJob: %v", err)
	}
	if got.ID != "job_2" {
		t.Errorf("GetJob id = %q, want job_2", got.ID)
	}
}

func TestAddJobDependencies_SendsDependsOn(t *testing.T) {
	var gotPath string
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		data, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(data, &gotBody); err != nil {
			t.Errorf("invalid body %s: %v", data, err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"dependencies_added":1,"dependencies_met":false}`))
	}))
	defer server.Close()

	res := NewWorkflowsResource(httpx.NewTransport(httpx.Config{BaseURL: server.URL, APIKey: "sp_test_key"}))
	out, err := res.AddJobDependencies(context.Background(), "job_2", &AddDependenciesRequest{
		DependsOn: []string{"job_1"},
	})
	if err != nil {
		t.Fatalf("AddJobDependencies: %v", err)
	}
	if gotPath != "/api/v1/jobs/job_2/dependencies" {
		t.Errorf("path = %q", gotPath)
	}
	got, _ := gotBody["depends_on"].([]any)
	if len(got) != 1 || got[0] != "job_1" {
		t.Errorf("depends_on = %v, want [job_1]", gotBody["depends_on"])
	}
	if out.DependenciesAdded != 1 {
		t.Errorf("DependenciesAdded = %d, want 1", out.DependenciesAdded)
	}
	if !out.Success {
		t.Error("Success = false, want true when dependencies_added > 0")
	}
}

func TestGetJobDependencies_UnmarshalsBackendShape(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/jobs/job_2/dependencies" {
			t.Errorf("path = %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"job_id":"job_2",
			"dependencies":[{"job_id":"job_1","queue_name":"etl","status":"completed"}],
			"dependents":[{"job_id":"job_3","queue_name":"etl","status":"pending"}],
			"dependencies_met":true
		}`))
	}))
	defer server.Close()

	res := NewWorkflowsResource(httpx.NewTransport(httpx.Config{BaseURL: server.URL, APIKey: "sp_test_key"}))
	got, err := res.GetJobDependencies(context.Background(), "job_2")
	if err != nil {
		t.Fatalf("GetJobDependencies: %v", err)
	}
	if got.JobID != "job_2" {
		t.Errorf("JobID = %q, want job_2", got.JobID)
	}
	if !got.DependenciesMet {
		t.Error("DependenciesMet = false, want true")
	}
	if len(got.Dependencies) != 1 || got.Dependencies[0].JobID != "job_1" {
		t.Errorf("Dependencies = %+v", got.Dependencies)
	}
	if len(got.Dependents) != 1 || got.Dependents[0].JobID != "job_3" {
		t.Errorf("Dependents = %+v", got.Dependents)
	}
}

// GET /workflows/{id} jobs carry timeout_ms, payload, result, error and
// attempt. ListJobs and GetJob used to keep only id, queue, status and timestamps.
func TestListJobs_KeepsTimeoutPayloadResultErrorAndAttempt(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/workflows/wf_1" {
			t.Errorf("path = %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"wf_1",
			"jobs":[
				{
					"id":"job_fail",
					"queue":"etl",
					"status":"failed",
					"payload":{"step":"charge"},
					"result":{"ok":false},
					"error":{"type":"JobError","message":"upstream 502"},
					"attempt":3,
					"timeout_ms":120000,
					"created_at":"2024-01-01T00:00:00Z"
				},
				{
					"id":"job_short",
					"queue":"etl",
					"status":"pending",
					"timeout_ms":500,
					"created_at":"2024-01-01T00:00:00Z"
				}
			],
			"dependencies":[
				{"parent_job_id":"job_short","child_job_id":"job_fail","dependency_type":"all"}
			]
		}`))
	}))
	defer server.Close()

	res := NewWorkflowsResource(httpx.NewTransport(httpx.Config{BaseURL: server.URL, APIKey: "sp_test_key"}))
	jobs, err := res.Jobs().ListJobs(context.Background(), "wf_1")
	if err != nil {
		t.Fatalf("ListJobs: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("len(jobs) = %d, want 2", len(jobs))
	}

	failed := jobs[0]
	if failed.TimeoutSeconds != 120 {
		t.Errorf("TimeoutSeconds = %d, want 120 (from timeout_ms 120000)", failed.TimeoutSeconds)
	}
	payload, _ := failed.Payload.(map[string]any)
	if payload["step"] != "charge" {
		t.Errorf("Payload = %#v, want step=charge", failed.Payload)
	}
	result, _ := failed.Result.(map[string]any)
	if result["ok"] != false {
		t.Errorf("Result = %#v, want ok=false", failed.Result)
	}
	if failed.Error == nil || failed.Error.Type != "JobError" || failed.Error.Message != "upstream 502" {
		t.Errorf("Error = %+v, want JobError upstream 502", failed.Error)
	}
	if failed.Attempt != 3 {
		t.Errorf("Attempt = %d, want 3", failed.Attempt)
	}
	if len(failed.DependsOn) != 1 || failed.DependsOn[0] != "job_short" {
		t.Errorf("DependsOn = %v, want [job_short]", failed.DependsOn)
	}
	if jobs[1].TimeoutSeconds != 1 {
		t.Errorf("sub-second timeout_ms TimeoutSeconds = %d, want 1", jobs[1].TimeoutSeconds)
	}
	if jobs[1].Payload != nil || jobs[1].Error != nil {
		t.Errorf("job without payload/error = payload %#v error %+v", jobs[1].Payload, jobs[1].Error)
	}

	got, err := res.Jobs().GetJob(context.Background(), "wf_1", "job_fail")
	if err != nil {
		t.Fatalf("GetJob: %v", err)
	}
	if got.TimeoutSeconds != 120 || got.Attempt != 3 || got.Error == nil || got.Error.Message != "upstream 502" {
		t.Errorf("GetJob = %+v, want timeout 120, attempt 3, error upstream 502", got)
	}
	gotPayload, _ := got.Payload.(map[string]any)
	if gotPayload["step"] != "charge" {
		t.Errorf("GetJob Payload = %#v", got.Payload)
	}
}
