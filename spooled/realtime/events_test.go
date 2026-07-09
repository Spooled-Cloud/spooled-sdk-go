package realtime

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// TestNormalizeEventType asserts every server event variant (PascalCase on the
// wire) maps to the SDK's canonical dotted EventType, that already-dotted and
// unknown values pass through unchanged, and that the SSE "job.status" alias
// converges on the same canonical type as the "JobStatusChange" body type.
func TestNormalizeEventType(t *testing.T) {
	cases := map[EventType]EventType{
		// PascalCase variants emitted by the backend (see realtime.rs).
		"JobStatusChange":    EventJobStatusChanged,
		"JobCreated":         EventJobCreated,
		"JobCompleted":       EventJobCompleted,
		"JobFailed":          EventJobFailed,
		"QueueStats":         EventQueueStats,
		"WorkerHeartbeat":    EventWorkerHeartbeat,
		"WorkerRegistered":   EventWorkerRegistered,
		"WorkerDeregistered": EventWorkerDeregistered,
		"SystemHealth":       EventSystemHealth,
		"Ping":               EventPing,
		"Error":              EventError,
		// SSE dotted event-line name for a status change.
		"job.status": EventJobStatusChanged,
		// Already-canonical dotted names pass through unchanged.
		"job.created":      EventJobCreated,
		"queue.stats":      EventQueueStats,
		"worker.heartbeat": EventWorkerHeartbeat,
		// Unknown / future types pass through so catch-all handlers still fire.
		"SomethingNew": "SomethingNew",
	}

	for in, want := range cases {
		if got := normalizeEventType(in); got != want {
			t.Errorf("normalizeEventType(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestWebSocketHandleMessage_DispatchesMappedTypes feeds raw wire messages (with
// PascalCase `type` and a nested `data` payload, exactly as the server emits)
// into handleMessage and asserts they reach both the catch-all handler (with a
// normalized type) and the correct typed handler.
func TestWebSocketHandleMessage_DispatchesMappedTypes(t *testing.T) {
	c := NewWebSocketClient(ConnectionOptions{WSURL: "ws://example.invalid/ws"})

	var allTypes []EventType
	c.OnEvent(func(e *Event) { allTypes = append(allTypes, e.Type) })

	var job *JobEvent
	c.OnJobEvent(EventJobCreated, func(e *JobEvent) { job = e })
	var statusChange *JobEvent
	c.OnJobEvent(EventJobStatusChanged, func(e *JobEvent) { statusChange = e })
	var queue *QueueEvent
	c.OnQueueEvent(EventQueueStats, func(e *QueueEvent) { queue = e })
	var worker *WorkerEvent
	c.OnWorkerEvent(EventWorkerHeartbeat, func(e *WorkerEvent) { worker = e })

	c.handleMessage([]byte(`{"type":"JobCreated","data":{"job_id":"job-1","queue_name":"emails","priority":5}}`))
	c.handleMessage([]byte(`{"type":"JobStatusChange","data":{"job_id":"job-1","queue_name":"emails","status":"processing"}}`))
	c.handleMessage([]byte(`{"type":"QueueStats","data":{"queue_name":"emails"}}`))
	c.handleMessage([]byte(`{"type":"WorkerHeartbeat","data":{"worker_id":"w-1"}}`))

	// Catch-all must see every event, with normalized (dotted) types.
	wantAll := []EventType{EventJobCreated, EventJobStatusChanged, EventQueueStats, EventWorkerHeartbeat}
	if len(allTypes) != len(wantAll) {
		t.Fatalf("catch-all saw %v, want %v", allTypes, wantAll)
	}
	for i, want := range wantAll {
		if allTypes[i] != want {
			t.Errorf("catch-all[%d] = %q, want %q", i, allTypes[i], want)
		}
	}

	// Typed handlers must fire with parsed payloads.
	if job == nil || job.JobID != "job-1" || job.QueueName != "emails" {
		t.Errorf("JobCreated handler got %+v, want job-1/emails", job)
	}
	if statusChange == nil || statusChange.Status != "processing" {
		t.Errorf("JobStatusChange handler got %+v, want status=processing", statusChange)
	}
	if queue == nil || queue.QueueName != "emails" {
		t.Errorf("QueueStats handler got %+v, want queue=emails", queue)
	}
	if worker == nil || worker.WorkerID != "w-1" {
		t.Errorf("WorkerHeartbeat handler got %+v, want worker=w-1", worker)
	}
}

// TestSSEHandleEvent_MapsPascalCase confirms the SSE transport also normalizes
// the PascalCase `type` carried in the event's JSON body before dispatch.
func TestSSEHandleEvent_MapsPascalCase(t *testing.T) {
	c := NewSSEClient(ConnectionOptions{BaseURL: "https://api.spooled.cloud"})

	var gotType EventType
	c.OnEvent(func(e *Event) { gotType = e.Type })
	var job *JobEvent
	c.OnJobEvent(EventJobCompleted, func(e *JobEvent) { job = e })

	// The backend labels the SSE `event:` line "job.completed" and repeats the
	// full RealtimeEvent JSON (PascalCase type) in the data body.
	c.handleSSEEvent("job.completed", `{"type":"JobCompleted","data":{"job_id":"job-9","queue_name":"emails"}}`)

	if gotType != EventJobCompleted {
		t.Errorf("catch-all type = %q, want %q", gotType, EventJobCompleted)
	}
	if job == nil || job.JobID != "job-9" {
		t.Errorf("JobCompleted handler got %+v, want job-9", job)
	}
}

// TestWebSocketSubscribe_SendsCmdShapeAndDoesNotBlock is a regression test for
// the shipped bug: Subscribe sent {"type":"subscribe",...} and then blocked up
// to 10s waiting for an ack the server never sends. It must now send
// {"cmd":"subscribe","queue","job_id"} and return immediately.
func TestWebSocketSubscribe_SendsCmdShapeAndDoesNotBlock(t *testing.T) {
	received := make(chan []byte, 2)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close(websocket.StatusNormalClosure, "")
		// Read the client's command but deliberately send NO acknowledgement.
		_, data, err := conn.Read(r.Context())
		if err != nil {
			return
		}
		received <- data
		// Keep the connection open briefly so the read above is not racing a close.
		time.Sleep(150 * time.Millisecond)
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws"
	c := NewWebSocketClient(ConnectionOptions{
		WSURL:         wsURL,
		Token:         "static-jwt", // static token => resolveToken skips login
		AutoReconnect: false,
	})
	if err := c.Connect(); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer c.Disconnect()

	start := time.Now()
	if err := c.Subscribe(SubscriptionFilter{QueueName: "emails", JobID: "job-1"}); err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("Subscribe blocked %v; it must not wait for an ack the server never sends", elapsed)
	}

	select {
	case data := <-received:
		var got map[string]any
		if err := json.Unmarshal(data, &got); err != nil {
			t.Fatalf("server received non-JSON command %q: %v", data, err)
		}
		if got["cmd"] != "subscribe" {
			t.Errorf(`command = %s, want cmd="subscribe"`, data)
		}
		if got["queue"] != "emails" || got["job_id"] != "job-1" {
			t.Errorf(`command = %s, want queue=emails job_id=job-1`, data)
		}
		if _, hasType := got["type"]; hasType {
			t.Errorf("command must not use the legacy \"type\" field: %s", data)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("server never received the subscribe command")
	}
}
