package realtime

import (
	"encoding/json"
	"testing"
	"time"
)

func TestEvent_TimestampComesFromData(t *testing.T) {
	var event Event
	body := `{"type":"JobCompleted","data":{"job_id":"j1","queue_name":"q","timestamp":"2026-11-16T10:20:30.5Z"}}`
	if err := json.Unmarshal([]byte(body), &event); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	want := time.Date(2026, 11, 16, 10, 20, 30, 500_000_000, time.UTC)
	if !event.Timestamp.Equal(want) {
		t.Errorf("Timestamp = %v, want %v", event.Timestamp, want)
	}
}

func TestEvent_TopLevelTimestampWins(t *testing.T) {
	var event Event
	body := `{"type":"JobCompleted","timestamp":"2026-01-01T00:00:00Z","data":{"timestamp":"2026-11-16T10:20:30Z"}}`
	if err := json.Unmarshal([]byte(body), &event); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if want := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC); !event.Timestamp.Equal(want) {
		t.Errorf("Timestamp = %v, want %v", event.Timestamp, want)
	}
}

func TestEvent_NoTimestampAnywhereStaysZero(t *testing.T) {
	for _, body := range []string{
		`{"type":"JobCompleted","data":{"job_id":"j1"}}`,
		`{"type":"Ping","data":"not an object"}`,
		`{"type":"Ping"}`,
	} {
		var event Event
		if err := json.Unmarshal([]byte(body), &event); err != nil {
			t.Fatalf("Unmarshal(%s): %v", body, err)
		}
		if !event.Timestamp.IsZero() {
			t.Errorf("Timestamp = %v for %s, want zero", event.Timestamp, body)
		}
	}
}
