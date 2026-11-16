package grpc

import (
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/spooled-cloud/spooled-sdk-go/spooled/grpc/pb"
	"github.com/spooled-cloud/spooled-sdk-go/spooled/resources"
)

func TestPbJobToJob_MapsEveryProtoField(t *testing.T) {
	created := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	started := created.Add(time.Minute)
	completed := created.Add(2 * time.Minute)
	scheduled := created.Add(-time.Hour)
	leaseExpires := created.Add(5 * time.Minute)
	payload, _ := structpb.NewStruct(map[string]any{"to": "a@example.com"})
	result, _ := structpb.NewStruct(map[string]any{"sent": true})

	got := pbJobToJob(&pb.Job{
		Id:               "job-1",
		OrganizationId:   "org-1",
		QueueName:        "emails",
		Status:           pb.JobStatus_JOB_STATUS_COMPLETED,
		Payload:          payload,
		Result:           result,
		RetryCount:       2,
		MaxRetries:       5,
		LastError:        "smtp timeout",
		Priority:         7,
		TimeoutSeconds:   60,
		CreatedAt:        timestamppb.New(created),
		ScheduledAt:      timestamppb.New(scheduled),
		StartedAt:        timestamppb.New(started),
		CompletedAt:      timestamppb.New(completed),
		LeaseExpiresAt:   timestamppb.New(leaseExpires),
		AssignedWorkerId: "worker-a",
		IdempotencyKey:   "idem-1",
		LeaseId:          "lease-1",
	})

	if got.ID != "job-1" || got.OrganizationID != "org-1" || got.QueueName != "emails" {
		t.Errorf("ids = %q/%q/%q", got.ID, got.OrganizationID, got.QueueName)
	}
	if got.Status != resources.JobStatusCompleted {
		t.Errorf("Status = %q, want completed", got.Status)
	}
	if got.Payload["to"] != "a@example.com" {
		t.Errorf("Payload = %v", got.Payload)
	}
	if got.Result["sent"] != true {
		t.Errorf("Result = %v, want sent=true", got.Result)
	}
	if got.RetryCount != 2 || got.MaxRetries != 5 || got.Priority != 7 || got.TimeoutSeconds != 60 {
		t.Errorf("counters = %d/%d/%d/%d", got.RetryCount, got.MaxRetries, got.Priority, got.TimeoutSeconds)
	}
	if got.LastError != "smtp timeout" {
		t.Errorf("LastError = %q", got.LastError)
	}
	for name, pair := range map[string]struct {
		got  *time.Time
		want time.Time
	}{
		"CreatedAt":      {got.CreatedAt, created},
		"ScheduledAt":    {got.ScheduledAt, scheduled},
		"StartedAt":      {got.StartedAt, started},
		"CompletedAt":    {got.CompletedAt, completed},
		"LeaseExpiresAt": {got.LeaseExpiresAt, leaseExpires},
	} {
		if pair.got == nil || !pair.got.Equal(pair.want) {
			t.Errorf("%s = %v, want %v", name, pair.got, pair.want)
		}
	}
	if got.AssignedWorkerID != "worker-a" || got.IdempotencyKey != "idem-1" || got.LeaseID != "lease-1" {
		t.Errorf("worker/idempotency/lease = %q/%q/%q", got.AssignedWorkerID, got.IdempotencyKey, got.LeaseID)
	}
}

func TestPbJobToJob_UnsetFieldsStayEmpty(t *testing.T) {
	got := pbJobToJob(&pb.Job{Id: "job-1"})
	if got.Status != "" {
		t.Errorf("Status = %q, want empty for JOB_STATUS_UNSPECIFIED", got.Status)
	}
	if got.Result != nil || got.CreatedAt != nil || got.CompletedAt != nil {
		t.Errorf("Result/CreatedAt/CompletedAt = %v/%v/%v, want nil", got.Result, got.CreatedAt, got.CompletedAt)
	}
	for _, want := range []resources.JobStatus{
		resources.JobStatusPending,
		resources.JobStatusScheduled,
		resources.JobStatusProcessing,
		resources.JobStatusCompleted,
		resources.JobStatusFailed,
		resources.JobStatusDeadletter,
		resources.JobStatusCancelled,
	} {
		value, ok := pb.JobStatus_value["JOB_STATUS_"+strings.ToUpper(string(want))]
		if !ok {
			t.Fatalf("proto has no status for %q", want)
		}
		if got := jobStatusFromProto(pb.JobStatus(value)); got != want {
			t.Errorf("jobStatusFromProto(%d) = %q, want %q", value, got, want)
		}
	}
	if got := jobStatusFromProto(pb.JobStatus(99)); got != "" {
		t.Errorf("unknown status = %q, want empty", got)
	}
}
