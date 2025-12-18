package realtime

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// SSEClient implements RealtimeClient using Server-Sent Events.
// Note: SSE is unidirectional - subscriptions must be set at connection time.
type SSEClient struct {
	opts              ConnectionOptions
	resp              *http.Response
	state             ConnectionState
	reconnectAttempts int
	filter            *SubscriptionFilter
	httpClient        *http.Client

	// Event handlers
	eventHandlers       map[EventType][]JobEventHandler
	queueEventHandlers  map[EventType][]QueueEventHandler
	workerEventHandlers map[EventType][]WorkerEventHandler
	allEventHandlers    []EventHandler
	stateChangeHandlers []StateChangeHandler

	mu     sync.RWMutex
	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}
}

// NewSSEClient creates a new SSE realtime client.
func NewSSEClient(opts ConnectionOptions) *SSEClient {
	defaults := DefaultConnectionOptions()
	if opts.BaseURL == "" {
		opts.BaseURL = defaults.BaseURL
	}
	if opts.ReconnectDelay == 0 {
		opts.ReconnectDelay = defaults.ReconnectDelay
	}
	if opts.MaxReconnectDelay == 0 {
		opts.MaxReconnectDelay = defaults.MaxReconnectDelay
	}
	if opts.MaxReconnectAttempts == 0 {
		opts.MaxReconnectAttempts = defaults.MaxReconnectAttempts
	}

	return &SSEClient{
		opts:                opts,
		state:               StateDisconnected,
		httpClient:          &http.Client{Timeout: 0}, // No timeout for SSE
		eventHandlers:       make(map[EventType][]JobEventHandler),
		queueEventHandlers:  make(map[EventType][]QueueEventHandler),
		workerEventHandlers: make(map[EventType][]WorkerEventHandler),
	}
}

// Connect establishes the SSE connection.
// For SSE, subscriptions must be provided at connect time via ConnectWithFilter.
func (c *SSEClient) Connect() error {
	return c.ConnectWithFilter(nil)
}

// ConnectWithFilter establishes the SSE connection with a subscription filter.
func (c *SSEClient) ConnectWithFilter(filter *SubscriptionFilter) error {
	c.mu.Lock()
	if c.state == StateConnected || c.state == StateConnecting {
		c.mu.Unlock()
		return nil
	}
	c.filter = filter
	c.setState(StateConnecting)
	c.mu.Unlock()

	return c.doConnect()
}

func (c *SSEClient) doConnect() error {
	sseURL := c.buildSSEURL()
	c.log("Connecting to SSE: %s", sseURL)

	req, err := http.NewRequest("GET", sseURL, nil)
	if err != nil {
		c.mu.Lock()
		c.setState(StateDisconnected)
		c.mu.Unlock()
		return fmt.Errorf("failed to create SSE request: %w", err)
	}

	// Add authentication headers
	if c.opts.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.opts.Token)
	} else if c.opts.APIKey != "" {
		req.Header.Set("X-API-Key", c.opts.APIKey)
	}

	// SSE-specific headers
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Connection", "keep-alive")

	c.mu.Lock()
	c.ctx, c.cancel = context.WithCancel(context.Background())
	c.mu.Unlock()

	req = req.WithContext(c.ctx)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.mu.Lock()
		c.setState(StateDisconnected)
		c.mu.Unlock()
		return fmt.Errorf("SSE connection failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		c.mu.Lock()
		c.setState(StateDisconnected)
		c.mu.Unlock()
		return fmt.Errorf("SSE connection failed with status: %d", resp.StatusCode)
	}

	c.mu.Lock()
	c.resp = resp
	c.done = make(chan struct{})
	c.reconnectAttempts = 0
	c.setState(StateConnected)
	c.mu.Unlock()

	// Start event reader
	go c.readLoop()

	return nil
}

// Disconnect closes the SSE connection.
func (c *SSEClient) Disconnect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.state == StateDisconnected {
		return nil
	}

	if c.cancel != nil {
		c.cancel()
	}

	if c.resp != nil {
		c.resp.Body.Close()
		c.resp = nil
	}

	c.setState(StateDisconnected)
	return nil
}

// State returns the current connection state.
func (c *SSEClient) State() ConnectionState {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.state
}

// Subscribe is not supported for SSE - use ConnectWithFilter instead.
func (c *SSEClient) Subscribe(filter SubscriptionFilter) error {
	return fmt.Errorf("SSE does not support runtime subscriptions; use ConnectWithFilter instead")
}

// Unsubscribe is not supported for SSE.
func (c *SSEClient) Unsubscribe(filter SubscriptionFilter) error {
	return fmt.Errorf("SSE does not support runtime unsubscriptions; reconnect with a new filter")
}

// OnEvent registers a handler for all events.
func (c *SSEClient) OnEvent(handler EventHandler) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.allEventHandlers = append(c.allEventHandlers, handler)
}

// OnJobEvent registers a handler for job events.
func (c *SSEClient) OnJobEvent(eventType EventType, handler JobEventHandler) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.eventHandlers[eventType] = append(c.eventHandlers[eventType], handler)
}

// OnQueueEvent registers a handler for queue events.
func (c *SSEClient) OnQueueEvent(eventType EventType, handler QueueEventHandler) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.queueEventHandlers[eventType] = append(c.queueEventHandlers[eventType], handler)
}

// OnWorkerEvent registers a handler for worker events.
func (c *SSEClient) OnWorkerEvent(eventType EventType, handler WorkerEventHandler) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.workerEventHandlers[eventType] = append(c.workerEventHandlers[eventType], handler)
}

// OnStateChange registers a handler for state changes.
func (c *SSEClient) OnStateChange(handler StateChangeHandler) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.stateChangeHandlers = append(c.stateChangeHandlers, handler)
}

func (c *SSEClient) readLoop() {
	defer func() {
		c.mu.Lock()
		if c.done != nil {
			close(c.done)
		}
		c.mu.Unlock()
	}()

	c.mu.RLock()
	resp := c.resp
	c.mu.RUnlock()

	if resp == nil {
		return
	}

	scanner := bufio.NewScanner(resp.Body)
	var eventType string
	var data strings.Builder

	for scanner.Scan() {
		line := scanner.Text()

		// Empty line signals end of event
		if line == "" {
			if data.Len() > 0 {
				c.handleSSEEvent(eventType, data.String())
				eventType = ""
				data.Reset()
			}
			continue
		}

		// Parse SSE fields
		if strings.HasPrefix(line, "event:") {
			eventType = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		} else if strings.HasPrefix(line, "data:") {
			if data.Len() > 0 {
				data.WriteString("\n")
			}
			data.WriteString(strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		} else if strings.HasPrefix(line, ":") {
			// Comment line, ignore (often used for keep-alive)
			c.log("SSE comment: %s", line)
		}
	}

	if err := scanner.Err(); err != nil {
		c.log("SSE read error: %v", err)
	}

	c.handleDisconnect()
}

func (c *SSEClient) handleSSEEvent(eventType string, data string) {
	c.log("Received SSE event: type=%s data=%s", eventType, data)

	// Parse the event data as JSON
	var event Event
	if err := json.Unmarshal([]byte(data), &event); err != nil {
		// If data is not valid JSON, try wrapping it
		event = Event{
			Type:      EventType(eventType),
			Timestamp: time.Now(),
			Data:      json.RawMessage(data),
		}
	}

	// If no event type from SSE, use the one from JSON
	if eventType != "" && event.Type == "" {
		event.Type = EventType(eventType)
	}

	c.dispatchEvent(&event)
}

func (c *SSEClient) dispatchEvent(event *Event) {
	c.mu.RLock()
	allHandlers := c.allEventHandlers
	jobHandlers := c.eventHandlers[event.Type]
	queueHandlers := c.queueEventHandlers[event.Type]
	workerHandlers := c.workerEventHandlers[event.Type]
	c.mu.RUnlock()

	// Call all-event handlers
	for _, handler := range allHandlers {
		func() {
			defer func() {
				if r := recover(); r != nil {
					c.log("Event handler panic: %v", r)
				}
			}()
			handler(event)
		}()
	}

	// Parse and dispatch typed events
	switch {
	case isJobEvent(event.Type):
		var jobEvent JobEvent
		if err := json.Unmarshal(event.Data, &jobEvent); err == nil {
			for _, handler := range jobHandlers {
				func() {
					defer func() {
						if r := recover(); r != nil {
							c.log("Job event handler panic: %v", r)
						}
					}()
					handler(&jobEvent)
				}()
			}
		}
	case isQueueEvent(event.Type):
		var queueEvent QueueEvent
		if err := json.Unmarshal(event.Data, &queueEvent); err == nil {
			for _, handler := range queueHandlers {
				func() {
					defer func() {
						if r := recover(); r != nil {
							c.log("Queue event handler panic: %v", r)
						}
					}()
					handler(&queueEvent)
				}()
			}
		}
	case isWorkerEvent(event.Type):
		var workerEvent WorkerEvent
		if err := json.Unmarshal(event.Data, &workerEvent); err == nil {
			for _, handler := range workerHandlers {
				func() {
					defer func() {
						if r := recover(); r != nil {
							c.log("Worker event handler panic: %v", r)
						}
					}()
					handler(&workerEvent)
				}()
			}
		}
	}
}

func (c *SSEClient) handleDisconnect() {
	c.mu.Lock()
	if c.state == StateDisconnected {
		c.mu.Unlock()
		return
	}

	if c.resp != nil {
		c.resp.Body.Close()
		c.resp = nil
	}

	if !c.opts.AutoReconnect {
		c.setState(StateDisconnected)
		c.mu.Unlock()
		return
	}

	c.reconnectAttempts++
	if c.opts.MaxReconnectAttempts > 0 && c.reconnectAttempts > c.opts.MaxReconnectAttempts {
		c.setState(StateDisconnected)
		c.mu.Unlock()
		c.log("Max reconnect attempts reached")
		return
	}

	c.setState(StateReconnecting)
	c.mu.Unlock()

	// Calculate backoff delay
	delay := c.opts.ReconnectDelay * time.Duration(1<<(c.reconnectAttempts-1))
	if delay > c.opts.MaxReconnectDelay {
		delay = c.opts.MaxReconnectDelay
	}

	c.log("Reconnecting in %v (attempt %d)", delay, c.reconnectAttempts)

	time.AfterFunc(delay, func() {
		if err := c.doConnect(); err != nil {
			c.log("Reconnect failed: %v", err)
			c.handleDisconnect()
		}
	})
}

func (c *SSEClient) buildSSEURL() string {
	baseURL := strings.TrimSuffix(c.opts.BaseURL, "/")
	sseURL := baseURL + "/api/v1/events"

	c.mu.RLock()
	filter := c.filter
	c.mu.RUnlock()

	if filter == nil {
		return sseURL
	}

	// Build query parameters from filter
	params := url.Values{}
	if filter.QueueName != "" {
		params.Set("queue", filter.QueueName)
	}
	if filter.JobID != "" {
		params.Set("job_id", filter.JobID)
	}
	if filter.WorkerID != "" {
		params.Set("worker_id", filter.WorkerID)
	}
	if len(filter.Events) > 0 {
		params.Set("events", strings.Join(filter.Events, ","))
	}

	if len(params) > 0 {
		sseURL += "?" + params.Encode()
	}

	return sseURL
}

func (c *SSEClient) setState(state ConnectionState) {
	if c.state == state {
		return
	}
	c.state = state

	// Make a copy of handlers to call outside the lock
	handlers := make([]StateChangeHandler, len(c.stateChangeHandlers))
	copy(handlers, c.stateChangeHandlers)

	// Call handlers outside the lock (we're already holding it)
	go func() {
		for _, handler := range handlers {
			func() {
				defer func() {
					if r := recover(); r != nil {
						c.log("State change handler panic: %v", r)
					}
				}()
				handler(state)
			}()
		}
	}()
}

func (c *SSEClient) log(format string, args ...any) {
	if c.opts.Logger != nil {
		c.opts.Logger(format, args...)
	} else if c.opts.Debug {
		fmt.Printf("[spooled-sse] "+format+"\n", args...)
	}
}
