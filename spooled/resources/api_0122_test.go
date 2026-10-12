package resources

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/spooled-cloud/spooled-sdk-go/internal/httpx"
)

// recordedRequest is what a fake API saw.
type recordedRequest struct {
	Method string
	Path   string
	Query  url.Values
	Body   map[string]any
}

// fakeServer answers every request with status, headers and body, and records
// the requests.
type fakeServer struct {
	mu       sync.Mutex
	requests []recordedRequest
	server   *httptest.Server
}

func newFakeServer(t *testing.T, headers map[string]string, body string) *fakeServer {
	t.Helper()
	f := &fakeServer{}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var decoded map[string]any
		_ = json.Unmarshal(raw, &decoded)
		f.mu.Lock()
		f.requests = append(f.requests, recordedRequest{Method: r.Method, Path: r.URL.Path, Query: r.URL.Query(), Body: decoded})
		f.mu.Unlock()
		for k, v := range headers {
			w.Header().Set(k, v)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(f.server.Close)
	return f
}

func (f *fakeServer) transport() *httpx.Transport {
	return httpx.NewTransport(httpx.Config{BaseURL: f.server.URL, APIKey: "sp_test_key"})
}

func (f *fakeServer) last(t *testing.T) recordedRequest {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.requests) == 0 {
		t.Fatal("no request reached the server")
	}
	return f.requests[len(f.requests)-1]
}

const twoJobs = `[{"id":"j1","queue_name":"q","status":"failed","attempt":1,"max_retries":3,"created_at":"2026-10-12T10:00:00Z"},` +
	`{"id":"j2","queue_name":"q","status":"deadletter","attempt":3,"max_retries":3,"created_at":"2026-10-12T10:00:01Z"}]`

func TestJobsListPage_SendsTheListFiltersAndReadsHasMore(t *testing.T) {
	f := newFakeServer(t, map[string]string{"X-Has-More": "true"}, twoJobs)
	res := NewJobsResource(f.transport())

	after := time.Date(2026, 10, 12, 12, 0, 0, 0, time.FixedZone("UTC+3", 3*3600))
	before := time.Date(2026, 10, 13, 0, 0, 0, 0, time.UTC)
	failed := JobStatusFailed
	page, err := res.ListPage(context.Background(), &ListJobsParams{
		QueueName:     strPtr("emails"),
		Status:        &failed,
		Statuses:      []JobStatus{JobStatusDeadletter},
		Tag:           strPtr("billing"),
		Q:             strPtr("send"),
		JobType:       strPtr("email.send"),
		CreatedAfter:  &after,
		CreatedBefore: &before,
		OrderBy:       strPtr("priority"),
		OrderDir:      strPtr("asc"),
		Limit:         intPtr(2),
		Offset:        intPtr(4),
	})
	if err != nil {
		t.Fatalf("ListPage: %v", err)
	}
	if !page.HasMore {
		t.Error("HasMore = false, want the X-Has-More: true header")
	}
	if len(page.Jobs) != 2 || page.Jobs[1].Status != JobStatusDeadletter || page.Jobs[1].RetryCount != 3 {
		t.Errorf("jobs = %+v", page.Jobs)
	}

	got := f.last(t)
	if got.Path != "/api/v1/jobs" {
		t.Errorf("path = %s", got.Path)
	}
	want := map[string]string{
		"queue_name":     "emails",
		"status":         "failed,deadletter",
		"tag":            "billing",
		"q":              "send",
		"job_type":       "email.send",
		"created_after":  "2026-10-12T09:00:00Z", // RFC 3339 in UTC
		"created_before": "2026-10-13T00:00:00Z",
		"order_by":       "priority",
		"order_dir":      "asc",
		"limit":          "2",
		"offset":         "4",
	}
	for key, value := range want {
		if got.Query.Get(key) != value {
			t.Errorf("query %s = %q, want %q", key, got.Query.Get(key), value)
		}
	}
	if len(got.Query) != len(want) {
		t.Errorf("query = %v, want exactly %v", got.Query, want)
	}

	// List keeps returning the bare slice.
	jobs, err := res.List(context.Background(), &ListJobsParams{Statuses: []JobStatus{JobStatusPending, JobStatusScheduled}})
	if err != nil || len(jobs) != 2 {
		t.Fatalf("List = %d jobs, %v", len(jobs), err)
	}
	if s := f.last(t).Query.Get("status"); s != "pending,scheduled" {
		t.Errorf("Statuses alone sent status=%q", s)
	}
}

func TestJobsListPage_WithoutHeaderHasNoMore(t *testing.T) {
	f := newFakeServer(t, nil, `[]`)
	page, err := NewJobsResource(f.transport()).ListPage(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListPage: %v", err)
	}
	if page.HasMore || len(page.Jobs) != 0 {
		t.Errorf("page = %+v, want empty and no more", page)
	}
	if len(f.last(t).Query) != 0 {
		t.Errorf("nil params sent query %v", f.last(t).Query)
	}
}

func TestDLQListPage_SendsFiltersAndReadsHasMore(t *testing.T) {
	f := newFakeServer(t, map[string]string{"X-Has-More": "true"}, twoJobs)
	since := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	page, err := NewJobsResource(f.transport()).DLQ().ListPage(context.Background(), &ListDLQParams{
		QueueName:    strPtr("emails"),
		JobType:      strPtr("email.send"),
		Q:            strPtr("j"),
		CreatedAfter: &since,
		Limit:        intPtr(2),
	})
	if err != nil {
		t.Fatalf("DLQ ListPage: %v", err)
	}
	if !page.HasMore || len(page.Jobs) != 2 {
		t.Errorf("page = %+v", page)
	}
	got := f.last(t)
	if got.Path != "/api/v1/jobs/dlq" || got.Query.Get("job_type") != "email.send" || got.Query.Get("q") != "j" ||
		got.Query.Get("created_after") != "2026-10-01T00:00:00Z" || got.Query.Get("queue_name") != "emails" || got.Query.Get("limit") != "2" {
		t.Errorf("request = %+v", got)
	}
	if got.Query.Has("status") {
		t.Error("the DLQ list must not send a status filter")
	}
}

func TestJobsHistory_ReadsEventsAndHasMore(t *testing.T) {
	f := newFakeServer(t, map[string]string{"X-Has-More": "true"},
		`[{"id":"h2","job_id":"j1","event_type":"completed","worker_id":"w-1","details":{"duration_ms":12},"created_at":"2026-10-12T10:00:02Z"},`+
			`{"id":"h1","job_id":"j1","event_type":"some.future.event","worker_id":null,"details":{},"created_at":"2026-10-12T10:00:00Z"}]`)
	page, err := NewJobsResource(f.transport()).History(context.Background(), "j1", &JobHistoryParams{
		Order: strPtr("desc"), Limit: intPtr(2), Offset: intPtr(0),
	})
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	got := f.last(t)
	if got.Method != http.MethodGet || got.Path != "/api/v1/jobs/j1/history" ||
		got.Query.Get("order_dir") != "desc" || got.Query.Get("limit") != "2" || got.Query.Get("offset") != "0" {
		t.Errorf("request = %+v", got)
	}
	if !page.HasMore || len(page.Events) != 2 {
		t.Fatalf("page = %+v", page)
	}
	first := page.Events[0]
	if first.EventType != "completed" || first.WorkerID == nil || *first.WorkerID != "w-1" || first.JobID != "j1" {
		t.Errorf("first event = %+v", first)
	}
	if details, _ := first.Details.(map[string]any); details["duration_ms"] != float64(12) {
		t.Errorf("details = %#v", first.Details)
	}
	if page.Events[1].EventType != "some.future.event" || page.Events[1].WorkerID != nil {
		t.Errorf("second event = %+v", page.Events[1])
	}

	empty := newFakeServer(t, nil, `[]`)
	page, err = NewJobsResource(empty.transport()).History(context.Background(), "j2", nil)
	if err != nil || page.Events == nil || len(page.Events) != 0 || page.HasMore {
		t.Errorf("empty history = %+v, %v", page, err)
	}
	if len(empty.last(t).Query) != 0 {
		t.Errorf("nil params sent query %v", empty.last(t).Query)
	}
}

func TestWorkers_DecodeJobCounters(t *testing.T) {
	detail := newFakeServer(t, nil, `{"id":"w1","organization_id":"o","queue_name":"q","queue_names":["q"],"hostname":"h",`+
		`"max_concurrency":5,"current_jobs":0,"status":"healthy","last_heartbeat":"2026-10-12T10:00:00Z","metadata":{},`+
		`"registered_at":"2026-10-12T09:00:00Z","updated_at":"2026-10-12T10:00:00Z","jobs_processed":41,"jobs_failed":3}`)
	worker, err := NewWorkersResource(detail.transport()).Get(context.Background(), "w1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if worker.JobsProcessed != 41 || worker.JobsFailed != 3 {
		t.Errorf("Get counters = %d/%d, want 41/3", worker.JobsProcessed, worker.JobsFailed)
	}

	list := newFakeServer(t, nil, `[{"id":"w1","queue_name":"q","hostname":"h","status":"hibernating","current_jobs":1,`+
		`"max_concurrency":5,"last_heartbeat":"2026-10-12T10:00:00Z","jobs_processed":12,"jobs_failed":1}]`)
	res := NewWorkersResource(list.transport())
	summaries, err := res.ListSummaries(context.Background())
	if err != nil || len(summaries) != 1 {
		t.Fatalf("ListSummaries = %v, %v", summaries, err)
	}
	if summaries[0].JobsProcessed != 12 || summaries[0].JobsFailed != 1 || summaries[0].Status != "hibernating" {
		t.Errorf("summary = %+v", summaries[0])
	}
	legacy, err := res.List(context.Background())
	if err != nil || len(legacy) != 1 || legacy[0].JobsProcessed != 12 || legacy[0].JobsFailed != 1 {
		t.Errorf("List = %+v, %v; the counters must carry over", legacy, err)
	}
}

const webhookJSON = `{"id":"wh1","organization_id":"o","name":"ops","url":"https://hooks.example.com/x",` +
	`"events":["job.deadlettered","job.retried","workflow.completed","workflow.failed","job.exploded"],` +
	`"format":"teams","enabled":true,"failure_count":0,"created_at":"2026-10-12T10:00:00Z","updated_at":"2026-10-12T10:00:00Z"}`

func TestWebhooks_FormatAndNewEvents(t *testing.T) {
	f := newFakeServer(t, nil, webhookJSON)
	res := NewWebhooksResource(f.transport())
	ctx := context.Background()

	teams := WebhookFormatTeams
	created, err := res.Create(ctx, &CreateOutgoingWebhookRequest{
		Name: "ops",
		URL:  "https://hooks.example.com/x",
		Events: []WebhookEvent{
			WebhookEventJobDeadlettered, WebhookEventJobRetried,
			WebhookEventWorkflowCompleted, WebhookEventWorkflowFailed,
		},
		Format: &teams,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	body := f.last(t).Body
	if body["format"] != "teams" {
		t.Errorf("create body format = %v, want teams", body["format"])
	}
	events, _ := body["events"].([]any)
	if len(events) != 4 || events[0] != "job.deadlettered" || events[1] != "job.retried" ||
		events[2] != "workflow.completed" || events[3] != "workflow.failed" {
		t.Errorf("create body events = %v", body["events"])
	}
	if created.Format != WebhookFormatTeams {
		t.Errorf("Format = %q, want teams", created.Format)
	}
	if len(created.Events) != 5 || created.Events[4] != "job.exploded" {
		t.Errorf("an unknown event did not decode as its name: %v", created.Events)
	}

	// Create without a format leaves the field out (the server defaults to json).
	if _, err := res.Create(ctx, &CreateOutgoingWebhookRequest{Name: "n", URL: "https://x.example", Events: []WebhookEvent{WebhookEventJobFailed}}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, ok := f.last(t).Body["format"]; ok {
		t.Error("Create without Format sent a format")
	}

	// Update with only a format sends only the format.
	slack := WebhookFormatSlack
	if _, err := res.Update(ctx, "wh1", &UpdateOutgoingWebhookRequest{Format: &slack}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if got := f.last(t).Body; len(got) != 1 || got["format"] != "slack" {
		t.Errorf("update body = %v, want only format=slack", got)
	}
	// Update without a format leaves it alone.
	if _, err := res.Update(ctx, "wh1", &UpdateOutgoingWebhookRequest{Name: strPtr("renamed")}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if _, ok := f.last(t).Body["format"]; ok {
		t.Error("Update without Format sent a format")
	}

	// A format the SDK does not know yet still decodes.
	future := newFakeServer(t, nil, `[`+strings.Replace(webhookJSON, `"teams"`, `"matrix"`, 1)+`]`)
	list, err := NewWebhooksResource(future.transport()).List(ctx)
	if err != nil || len(list) != 1 || list[0].Format != "matrix" {
		t.Errorf("List with an unknown format = %+v, %v", list, err)
	}
}

func TestIngestListDeliveries_SendsFiltersAndDecodesRows(t *testing.T) {
	f := newFakeServer(t, nil, `[{"id":"d2","provider":"custom","event_type":"order.paid","matched_queue":"orders",`+
		`"job_id":"j9","status":"duplicate","created_at":"2026-10-12T10:00:00Z"},`+
		`{"id":"d1","provider":"custom","event_type":"order.paid","matched_queue":null,"job_id":null,"status":null,`+
		`"created_at":"2026-10-01T10:00:00Z"}]`)
	since := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	dup := InboundDeliveryDuplicate
	rows, err := NewIngestResource(f.transport()).ListDeliveries(context.Background(), &ListInboundDeliveriesParams{
		Status: &dup, Since: &since, QueueName: strPtr("orders"), Limit: intPtr(10), Offset: intPtr(20),
	})
	if err != nil {
		t.Fatalf("ListDeliveries: %v", err)
	}
	got := f.last(t)
	if got.Method != http.MethodGet || got.Path != "/api/v1/webhooks/deliveries" {
		t.Errorf("request = %s %s", got.Method, got.Path)
	}
	want := map[string]string{"status": "duplicate", "since": "2026-10-01T00:00:00Z", "queue_name": "orders", "limit": "10", "offset": "20"}
	for key, value := range want {
		if got.Query.Get(key) != value {
			t.Errorf("query %s = %q, want %q", key, got.Query.Get(key), value)
		}
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %+v", rows)
	}
	if rows[0].JobID == nil || *rows[0].JobID != "j9" || rows[0].Status == nil || *rows[0].Status != InboundDeliveryDuplicate ||
		rows[0].MatchedQueue == nil || *rows[0].MatchedQueue != "orders" {
		t.Errorf("row 0 = %+v", rows[0])
	}
	if rows[1].JobID != nil || rows[1].Status != nil || rows[1].MatchedQueue != nil {
		t.Errorf("row 1 (before 0.1.122) = %+v, want nil job, status and queue", rows[1])
	}
}

// TestResponseModels_TolerateUnknownEnumValues: a value the API adds later
// (a job status, plan tier, workflow or worker status, schedule run status)
// must decode as its name, never fail the call.
func TestResponseModels_TolerateUnknownEnumValues(t *testing.T) {
	ctx := context.Background()

	job := newFakeServer(t, nil, `{"id":"j1","organization_id":"o","queue_name":"q","status":"paused","payload":{},`+
		`"retry_count":0,"max_retries":3,"created_at":"2026-10-12T10:00:00Z","priority":0,"timeout_seconds":30,"updated_at":"2026-10-12T10:00:00Z"}`)
	gotJob, err := NewJobsResource(job.transport()).Get(ctx, "j1")
	if err != nil || gotJob.Status != "paused" {
		t.Errorf("job with an unknown status = %+v, %v", gotJob, err)
	}

	wf := newFakeServer(t, nil, `[{"id":"wf1","organization_id":"o","name":"n","status":"archived","total_jobs":1,`+
		`"completed_jobs":0,"failed_jobs":0,"created_at":"2026-10-12T10:00:00Z"}]`)
	workflows, err := NewWorkflowsResource(wf.transport()).List(ctx, nil)
	if err != nil || len(workflows) != 1 || workflows[0].Status != "archived" {
		t.Errorf("workflow with an unknown status = %+v, %v", workflows, err)
	}

	org := newFakeServer(t, nil, `{"id":"o","name":"n","slug":"s","plan_tier":"scale","settings":{},`+
		`"created_at":"2026-10-12T10:00:00Z","updated_at":"2026-10-12T10:00:00Z"}`)
	gotOrg, err := NewOrganizationsResource(org.transport()).Get(ctx, "o")
	if err != nil || gotOrg.PlanTier != "scale" {
		t.Errorf("organization with an unknown plan tier = %+v, %v", gotOrg, err)
	}

	deliveries := newFakeServer(t, nil, `[{"id":"d","webhook_id":"wh","event":"workflow.paused","payload":{},"status":"throttled",`+
		`"attempts":1,"created_at":"2026-10-12T10:00:00Z"}]`)
	rows, err := NewWebhooksResource(deliveries.transport()).Deliveries(ctx, "wh", nil)
	if err != nil || len(rows) != 1 || rows[0].Event != "workflow.paused" || rows[0].Status != "throttled" {
		t.Errorf("delivery with unknown event and status = %+v, %v", rows, err)
	}
}

func TestCreateWorkflowResponse_MapsKeysToJobIDs(t *testing.T) {
	created := &CreateWorkflowResponse{
		WorkflowID: "wf1",
		JobIDs:     []WorkflowJobMapping{{Key: "extract", JobID: "j1"}, {Key: "load", JobID: "j2"}},
	}
	if keys := created.KeyByJobID(); len(keys) != 2 || keys["j1"] != "extract" || keys["j2"] != "load" {
		t.Errorf("KeyByJobID = %v", keys)
	}
	if id, ok := created.JobID("load"); !ok || id != "j2" {
		t.Errorf("JobID(load) = %q, %v", id, ok)
	}
	if _, ok := created.JobID("missing"); ok {
		t.Error("JobID found a key that was never created")
	}

	// Workflow details carry no keys (see ListJobs); ApplyKeys fills them.
	f := newFakeServer(t, nil, `{"id":"wf1","jobs":[{"id":"j1","queue":"etl","status":"completed","created_at":"2026-10-12T10:00:00Z"},`+
		`{"id":"j2","queue":"etl","status":"pending","created_at":"2026-10-12T10:00:00Z"},`+
		`{"id":"j3","queue":"etl","status":"pending","created_at":"2026-10-12T10:00:00Z"}],`+
		`"dependencies":[{"parent_job_id":"j1","child_job_id":"j2"}]}`)
	jobs, err := NewWorkflowsResource(f.transport()).Jobs().ListJobs(context.Background(), "wf1")
	if err != nil || len(jobs) != 3 {
		t.Fatalf("ListJobs = %+v, %v", jobs, err)
	}
	if jobs[0].Key != "" {
		t.Errorf("Key = %q before ApplyKeys; the API sends none", jobs[0].Key)
	}
	// An empty key is left out when a WorkflowJob is encoded again.
	encoded, err := json.Marshal(jobs[0])
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(encoded), `"key"`) {
		t.Errorf("an unknown key was encoded as %s", encoded)
	}

	created.ApplyKeys(jobs)
	if jobs[0].Key != "extract" || jobs[1].Key != "load" || jobs[2].Key != "" {
		t.Errorf("keys after ApplyKeys = %q, %q, %q", jobs[0].Key, jobs[1].Key, jobs[2].Key)
	}
	if len(jobs[1].DependsOn) != 1 || jobs[1].DependsOn[0] != "j1" {
		t.Errorf("DependsOn = %v", jobs[1].DependsOn)
	}
}
