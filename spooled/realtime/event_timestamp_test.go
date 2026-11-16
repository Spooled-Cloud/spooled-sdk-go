package realtime

import (
	"encoding/json"
	"testing"
	"time"
)

func TestEvent_TimestampComesFromData(t *testing.T) {
	body := `{"type":"JobCompleted","data":{"job_id":"j1","queue_name":"q","timestamp":"2026-11-16T10:20:30.5Z"}}`
	event, err := decodeEvent([]byte(body))
	if err != nil {
		t.Fatalf("decodeEvent: %v", err)
	}
	want := time.Date(2026, 11, 16, 10, 20, 30, 500_000_000, time.UTC)
	if !event.Timestamp.Equal(want) {
		t.Errorf("Timestamp = %v, want %v", event.Timestamp, want)
	}
}

func TestEvent_TopLevelTimestampWins(t *testing.T) {
	body := `{"type":"JobCompleted","timestamp":"2026-01-01T00:00:00Z","data":{"timestamp":"2026-11-16T10:20:30Z"}}`
	event, err := decodeEvent([]byte(body))
	if err != nil {
		t.Fatalf("decodeEvent: %v", err)
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
		event, err := decodeEvent([]byte(body))
		if err != nil {
			t.Fatalf("decodeEvent(%s): %v", body, err)
		}
		if !event.Timestamp.IsZero() {
			t.Errorf("Timestamp = %v for %s, want zero", event.Timestamp, body)
		}
	}
}

func TestEvent_ClientsFillTimestampFromData(t *testing.T) {
	const body = `{"type":"JobCompleted","data":{"job_id":"j1","queue_name":"q","timestamp":"2026-11-16T10:20:30Z"}}`
	want := time.Date(2026, 11, 16, 10, 20, 30, 0, time.UTC)

	ws := NewWebSocketClient(ConnectionOptions{WSURL: "ws://example.invalid/ws"})
	var fromWS *Event
	ws.OnEvent(func(e *Event) { fromWS = e })
	ws.handleMessage([]byte(body))

	sse := NewSSEClient(ConnectionOptions{BaseURL: "https://api.spooled.cloud"})
	var fromSSE *Event
	sse.OnEvent(func(e *Event) { fromSSE = e })
	sse.handleSSEEvent("job.completed", body)

	for name, got := range map[string]*Event{"websocket": fromWS, "sse": fromSSE} {
		if got == nil {
			t.Errorf("%s: OnEvent never fired", name)
			continue
		}
		if !got.Timestamp.Equal(want) {
			t.Errorf("%s: Timestamp = %v, want %v", name, got.Timestamp, want)
		}
	}
}

// Event has no UnmarshalJSON method, so a struct that embeds it decodes its
// own fields too (a promoted method would take over and drop them).
func TestEvent_EmbeddingStructDecodesItsOwnFields(t *testing.T) {
	var wrapped struct {
		Event
		Source string `json:"source"`
	}
	body := `{"type":"job.completed","data":{"job_id":"j1"},"source":"replay"}`
	if err := json.Unmarshal([]byte(body), &wrapped); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if wrapped.Source != "replay" || wrapped.Type != EventJobCompleted {
		t.Errorf("Source = %q, Type = %q; want replay, job.completed", wrapped.Source, wrapped.Type)
	}
}
