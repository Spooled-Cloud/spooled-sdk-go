package realtime

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func waitUntil(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// sseServer serves `handle` for every connection to /api/v1/events and counts
// the connections.
func sseServer(t *testing.T, handle func(w http.ResponseWriter, r *http.Request, conn int)) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var conns atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := int(conns.Add(1))
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		handle(w, r, n)
	}))
	t.Cleanup(server.Close)
	return server, &conns
}

func writeSSE(w http.ResponseWriter, event, data string) {
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data)
	w.(http.Flusher).Flush()
}

// --- SDKGP-08: JobEvent status ----------------------------------------------

func TestJobEvent_StatusComesFromNewStatus(t *testing.T) {
	// Exactly the data the backend sends for a status change (realtime.rs
	// RealtimeEvent::JobStatusChange): old_status/new_status, no "status".
	c := NewWebSocketClient(ConnectionOptions{WSURL: "ws://example.invalid/ws"})
	var got *JobEvent
	c.OnJobEvent(EventJobStatusChanged, func(e *JobEvent) { got = e })

	c.handleMessage([]byte(`{"type":"JobStatusChange","data":{"job_id":"j1","queue_name":"emails",` +
		`"old_status":"pending","new_status":"processing","timestamp":"2026-10-10T12:00:00Z"}}`))

	if got == nil {
		t.Fatal("status change handler never fired")
	}
	if got.Status != "processing" || got.OldStatus != "pending" || got.NewStatus != "processing" {
		t.Errorf("status = %q old=%q new=%q, want processing/pending/processing", got.Status, got.OldStatus, got.NewStatus)
	}
	if got.Timestamp.IsZero() {
		t.Error("timestamp was not decoded")
	}
}

func TestJobEvent_NonObjectResultDoesNotDropTheEvent(t *testing.T) {
	c := NewSSEClient(ConnectionOptions{BaseURL: "https://api.spooled.cloud"})
	var events []*JobEvent
	c.OnJobEvent(EventJobCompleted, func(e *JobEvent) { events = append(events, e) })

	c.handleSSEEvent("job.completed", `{"type":"JobCompleted","data":{"job_id":"a","queue_name":"q","duration_ms":12,"result":[1,2,3]}}`)
	c.handleSSEEvent("job.completed", `{"type":"JobCompleted","data":{"job_id":"b","queue_name":"q","duration_ms":3,"result":"done"}}`)
	c.handleSSEEvent("job.completed", `{"type":"JobCompleted","data":{"job_id":"c","queue_name":"q","result":{"rows":4}}}`)

	if len(events) != 3 {
		t.Fatalf("handlers fired %d times, want 3: a non-object result used to make the whole event disappear", len(events))
	}
	if arr, ok := events[0].RawResult.([]any); !ok || len(arr) != 3 || events[0].Result != nil || events[0].DurationMs != 12 {
		t.Errorf("array result: %+v", events[0])
	}
	if events[1].RawResult != "done" {
		t.Errorf("string result: %+v", events[1])
	}
	if events[2].Result["rows"] != float64(4) || events[2].RawResult == nil {
		t.Errorf("object result: %+v", events[2])
	}
}

// --- SDKGP-18: no credential in the URL -------------------------------------

// (see TestSSE_APIKeyTravelsInAuthorizationHeaderNotURL in realtime_test.go)

// --- SDKGP-07: read limits ---------------------------------------------------

func TestSSE_LargeEventIsDeliveredAndKeepsTheConnection(t *testing.T) {
	big := strings.Repeat("x", 300*1024) // far above the 64 KiB scanner default
	server, conns := sseServer(t, func(w http.ResponseWriter, r *http.Request, n int) {
		writeSSE(w, "job.completed", `{"type":"JobCompleted","data":{"job_id":"big","queue_name":"q","result":{"blob":"`+big+`"}}}`)
		<-r.Context().Done()
	})

	c := NewSSEClient(ConnectionOptions{BaseURL: server.URL, APIKey: "sp_live_x", DisableAutoReconnect: true})
	got := make(chan *JobEvent, 1)
	c.OnJobEvent(EventJobCompleted, func(e *JobEvent) { got <- e })
	if err := c.Connect(); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer c.Disconnect()

	select {
	case e := <-got:
		if blob, _ := e.Result["blob"].(string); len(blob) != len(big) {
			t.Errorf("blob length = %d, want %d", len(blob), len(big))
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the large event was never delivered")
	}
	if c.State() != StateConnected {
		t.Errorf("state = %s after a large event, want connected", c.State())
	}
	if conns.Load() != 1 {
		t.Errorf("connections = %d, want 1", conns.Load())
	}
}

func TestSSE_MaxMessageBytesIsConfigurable(t *testing.T) {
	server, _ := sseServer(t, func(w http.ResponseWriter, r *http.Request, n int) {
		writeSSE(w, "job.completed", `{"type":"JobCompleted","data":{"job_id":"big","result":"`+strings.Repeat("y", 4000)+`"}}`)
		<-r.Context().Done()
	})
	c := NewSSEClient(ConnectionOptions{BaseURL: server.URL, APIKey: "sp_live_x", DisableAutoReconnect: true, MaxMessageBytes: 1024})
	if err := c.Connect(); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer c.Disconnect()
	waitUntil(t, "the oversized event to drop the connection", func() bool { return c.State() == StateDisconnected })
}

func TestWebSocket_LargeMessageIsDelivered(t *testing.T) {
	big := strings.Repeat("z", 300*1024) // coder/websocket reads at most 32 KiB by default
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close(websocket.StatusNormalClosure, "")
		msg := `{"type":"JobCompleted","data":{"job_id":"big","queue_name":"q","result":{"blob":"` + big + `"}}}`
		if err := conn.Write(r.Context(), websocket.MessageText, []byte(msg)); err != nil {
			return
		}
		<-r.Context().Done()
	}))
	defer server.Close()

	c := NewWebSocketClient(ConnectionOptions{
		WSURL: "ws" + strings.TrimPrefix(server.URL, "http") + "/ws", Token: "static-jwt", DisableAutoReconnect: true,
	})
	got := make(chan *JobEvent, 1)
	c.OnJobEvent(EventJobCompleted, func(e *JobEvent) { got <- e })
	if err := c.Connect(); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer c.Disconnect()

	select {
	case e := <-got:
		if blob, _ := e.Result["blob"].(string); len(blob) != len(big) {
			t.Errorf("blob length = %d, want %d", len(blob), len(big))
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the large message was never delivered (read limit)")
	}
}

// --- SDKGP-06: reconnect by default -----------------------------------------

func TestSSE_ReconnectsByDefaultAfterTheServerClosesTheStream(t *testing.T) {
	server, conns := sseServer(t, func(w http.ResponseWriter, r *http.Request, n int) {
		writeSSE(w, "job.created", fmt.Sprintf(`{"type":"JobCreated","data":{"job_id":"j%d","queue_name":"q"}}`, n))
		// Returning ends the response: the server closing the stream, as it does
		// after 30 minutes.
	})

	// Zero value for every reconnect option except a fast delay: the shape of a
	// client from Client.RealtimeSSE().
	c := NewSSEClient(ConnectionOptions{BaseURL: server.URL, APIKey: "sp_live_x", ReconnectDelay: 5 * time.Millisecond})
	var mu sync.Mutex
	seen := map[string]bool{}
	c.OnJobEvent(EventJobCreated, func(e *JobEvent) {
		mu.Lock()
		seen[e.JobID] = true
		mu.Unlock()
	})
	if err := c.Connect(); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer c.Disconnect()

	waitUntil(t, "a second connection and its event", func() bool {
		mu.Lock()
		defer mu.Unlock()
		return conns.Load() >= 2 && seen["j1"] && seen["j2"]
	})
}

func TestSSE_DisableAutoReconnectStaysDisconnected(t *testing.T) {
	server, conns := sseServer(t, func(w http.ResponseWriter, r *http.Request, n int) {})
	c := NewSSEClient(ConnectionOptions{BaseURL: server.URL, APIKey: "sp_live_x", ReconnectDelay: 5 * time.Millisecond, DisableAutoReconnect: true})
	if err := c.Connect(); err != nil {
		t.Fatalf("connect: %v", err)
	}
	waitUntil(t, "disconnect", func() bool { return c.State() == StateDisconnected })
	time.Sleep(100 * time.Millisecond)
	if conns.Load() != 1 {
		t.Errorf("connections = %d, want 1 when auto-reconnect is disabled", conns.Load())
	}
}

func TestWebSocket_ReconnectsByDefaultAfterTheServerClosesTheSocket(t *testing.T) {
	var conns atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		conns.Add(1)
		_ = conn.Close(websocket.StatusGoingAway, "30 minute limit")
	}))
	defer server.Close()

	c := NewWebSocketClient(ConnectionOptions{
		WSURL: "ws" + strings.TrimPrefix(server.URL, "http") + "/ws", Token: "static-jwt", ReconnectDelay: 5 * time.Millisecond,
	})
	if err := c.Connect(); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer c.Disconnect()
	waitUntil(t, "a reconnect", func() bool { return conns.Load() >= 2 })
}

func TestConnectionOptions_AutoReconnectDefaultsOn(t *testing.T) {
	var zero ConnectionOptions
	zero.applyDefaults()
	if !zero.AutoReconnect {
		t.Error("a zero ConnectionOptions must reconnect")
	}
	if zero.MaxMessageBytes != DefaultMaxMessageBytes || DefaultMaxMessageBytes < 4<<20 {
		t.Errorf("MaxMessageBytes = %d, want at least 4 MiB", zero.MaxMessageBytes)
	}
	off := ConnectionOptions{DisableAutoReconnect: true}
	off.applyDefaults()
	if off.AutoReconnect {
		t.Error("DisableAutoReconnect must turn reconnecting off")
	}
}

// A server that accepts and immediately closes must not reconnect forever: the
// failure streak only resets after a connection stayed up.
func TestSSE_MaxReconnectAttemptsTripsWhenConnectionsDieImmediately(t *testing.T) {
	server, conns := sseServer(t, func(w http.ResponseWriter, r *http.Request, n int) {})
	c := NewSSEClient(ConnectionOptions{
		BaseURL: server.URL, APIKey: "sp_live_x",
		ReconnectDelay: 2 * time.Millisecond, MaxReconnectDelay: 5 * time.Millisecond, MaxReconnectAttempts: 3,
	})
	if err := c.Connect(); err != nil {
		t.Fatalf("connect: %v", err)
	}
	waitUntil(t, "the client to give up", func() bool { return c.State() == StateDisconnected })
	if n := conns.Load(); n < 2 || n > 6 {
		t.Errorf("connections = %d, want a handful bounded by MaxReconnectAttempts=3", n)
	}
}

// --- SSE parser: an event without data must not leak its type ---------------

func TestSSE_EventTypeDoesNotLeakAcrossDatalessEvents(t *testing.T) {
	server, _ := sseServer(t, func(w http.ResponseWriter, r *http.Request, n int) {
		// "event:" with no data line, then a data-only event.
		fmt.Fprint(w, "event: job.created\n\n")
		fmt.Fprint(w, "data: {\"data\":{\"job_id\":\"x\"}}\n\n")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	})
	c := NewSSEClient(ConnectionOptions{BaseURL: server.URL, APIKey: "sp_live_x", DisableAutoReconnect: true})
	got := make(chan EventType, 1)
	c.OnEvent(func(e *Event) { got <- e.Type })
	if err := c.Connect(); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer c.Disconnect()

	select {
	case typ := <-got:
		if typ != "" {
			t.Errorf("event type = %q; the dataless event's type leaked onto the next event", typ)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no event delivered")
	}
}
