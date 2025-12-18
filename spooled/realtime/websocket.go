package realtime

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"nhooyr.io/websocket"
)

// WebSocketClient implements RealtimeClient using WebSocket.
type WebSocketClient struct {
	opts              ConnectionOptions
	conn              *websocket.Conn
	state             ConnectionState
	reconnectAttempts int
	subscriptions     map[string]SubscriptionFilter
	pendingCommands   map[string]chan error

	// Event handlers
	eventHandlers       map[EventType][]JobEventHandler
	queueEventHandlers  map[EventType][]QueueEventHandler
	workerEventHandlers map[EventType][]WorkerEventHandler
	allEventHandlers    []EventHandler
	stateChangeHandlers []StateChangeHandler

	mu       sync.RWMutex
	cmdMu    sync.Mutex
	ctx      context.Context
	cancel   context.CancelFunc
	done     chan struct{}
	cmdSeq   int
}

// NewWebSocketClient creates a new WebSocket realtime client.
func NewWebSocketClient(opts ConnectionOptions) *WebSocketClient {
	defaults := DefaultConnectionOptions()
	if opts.WSURL == "" {
		opts.WSURL = defaults.WSURL
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

	return &WebSocketClient{
		opts:                opts,
		state:               StateDisconnected,
		subscriptions:       make(map[string]SubscriptionFilter),
		pendingCommands:     make(map[string]chan error),
		eventHandlers:       make(map[EventType][]JobEventHandler),
		queueEventHandlers:  make(map[EventType][]QueueEventHandler),
		workerEventHandlers: make(map[EventType][]WorkerEventHandler),
	}
}

// Connect establishes the WebSocket connection.
func (c *WebSocketClient) Connect() error {
	c.mu.Lock()
	if c.state == StateConnected || c.state == StateConnecting {
		c.mu.Unlock()
		return nil
	}
	c.setState(StateConnecting)
	c.mu.Unlock()

	return c.doConnect()
}

func (c *WebSocketClient) doConnect() error {
	c.log("Connecting to WebSocket: %s", c.opts.WSURL)

	// Build headers for authentication
	headers := http.Header{}
	if c.opts.Token != "" {
		headers.Set("Authorization", "Bearer "+c.opts.Token)
	} else if c.opts.APIKey != "" {
		headers.Set("X-API-Key", c.opts.APIKey)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	conn, _, err := websocket.Dial(ctx, c.opts.WSURL, &websocket.DialOptions{
		HTTPHeader: headers,
	})
	if err != nil {
		c.mu.Lock()
		c.setState(StateDisconnected)
		c.mu.Unlock()
		return fmt.Errorf("websocket dial failed: %w", err)
	}

	c.mu.Lock()
	c.conn = conn
	c.ctx, c.cancel = context.WithCancel(context.Background())
	c.done = make(chan struct{})
	c.reconnectAttempts = 0
	c.setState(StateConnected)
	c.mu.Unlock()

	// Start message reader
	go c.readLoop()

	// Resubscribe to previous subscriptions
	c.mu.RLock()
	subs := make([]SubscriptionFilter, 0, len(c.subscriptions))
	for _, sub := range c.subscriptions {
		subs = append(subs, sub)
	}
	c.mu.RUnlock()

	for _, sub := range subs {
		if err := c.Subscribe(sub); err != nil {
			c.log("Failed to resubscribe: %v", err)
		}
	}

	return nil
}

// Disconnect closes the WebSocket connection.
func (c *WebSocketClient) Disconnect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.state == StateDisconnected {
		return nil
	}

	if c.cancel != nil {
		c.cancel()
	}

	if c.conn != nil {
		err := c.conn.Close(websocket.StatusNormalClosure, "client disconnect")
		c.conn = nil
		c.setState(StateDisconnected)
		return err
	}

	c.setState(StateDisconnected)
	return nil
}

// State returns the current connection state.
func (c *WebSocketClient) State() ConnectionState {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.state
}

// Subscribe adds a subscription filter.
func (c *WebSocketClient) Subscribe(filter SubscriptionFilter) error {
	c.cmdMu.Lock()
	c.cmdSeq++
	requestID := fmt.Sprintf("sub-%d", c.cmdSeq)
	c.cmdMu.Unlock()

	cmd := wsCommand{
		Type:      "subscribe",
		RequestID: requestID,
		Filter:    &filter,
	}

	respCh := make(chan error, 1)
	c.cmdMu.Lock()
	c.pendingCommands[requestID] = respCh
	c.cmdMu.Unlock()

	defer func() {
		c.cmdMu.Lock()
		delete(c.pendingCommands, requestID)
		c.cmdMu.Unlock()
	}()

	data, err := json.Marshal(cmd)
	if err != nil {
		return fmt.Errorf("failed to marshal subscribe command: %w", err)
	}

	c.mu.RLock()
	conn := c.conn
	ctx := c.ctx
	c.mu.RUnlock()

	if conn == nil {
		return fmt.Errorf("not connected")
	}

	if err := conn.Write(ctx, websocket.MessageText, data); err != nil {
		return fmt.Errorf("failed to send subscribe command: %w", err)
	}

	// Wait for response with timeout
	select {
	case err := <-respCh:
		if err != nil {
			return err
		}
		// Store subscription
		key := subscriptionKey(filter)
		c.mu.Lock()
		c.subscriptions[key] = filter
		c.mu.Unlock()
		return nil
	case <-time.After(10 * time.Second):
		return fmt.Errorf("subscribe timeout")
	}
}

// Unsubscribe removes a subscription.
func (c *WebSocketClient) Unsubscribe(filter SubscriptionFilter) error {
	c.cmdMu.Lock()
	c.cmdSeq++
	requestID := fmt.Sprintf("unsub-%d", c.cmdSeq)
	c.cmdMu.Unlock()

	cmd := wsCommand{
		Type:      "unsubscribe",
		RequestID: requestID,
		Filter:    &filter,
	}

	respCh := make(chan error, 1)
	c.cmdMu.Lock()
	c.pendingCommands[requestID] = respCh
	c.cmdMu.Unlock()

	defer func() {
		c.cmdMu.Lock()
		delete(c.pendingCommands, requestID)
		c.cmdMu.Unlock()
	}()

	data, err := json.Marshal(cmd)
	if err != nil {
		return fmt.Errorf("failed to marshal unsubscribe command: %w", err)
	}

	c.mu.RLock()
	conn := c.conn
	ctx := c.ctx
	c.mu.RUnlock()

	if conn == nil {
		return fmt.Errorf("not connected")
	}

	if err := conn.Write(ctx, websocket.MessageText, data); err != nil {
		return fmt.Errorf("failed to send unsubscribe command: %w", err)
	}

	// Wait for response with timeout
	select {
	case err := <-respCh:
		if err != nil {
			return err
		}
		// Remove subscription
		key := subscriptionKey(filter)
		c.mu.Lock()
		delete(c.subscriptions, key)
		c.mu.Unlock()
		return nil
	case <-time.After(10 * time.Second):
		return fmt.Errorf("unsubscribe timeout")
	}
}

// OnEvent registers a handler for all events.
func (c *WebSocketClient) OnEvent(handler EventHandler) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.allEventHandlers = append(c.allEventHandlers, handler)
}

// OnJobEvent registers a handler for job events.
func (c *WebSocketClient) OnJobEvent(eventType EventType, handler JobEventHandler) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.eventHandlers[eventType] = append(c.eventHandlers[eventType], handler)
}

// OnQueueEvent registers a handler for queue events.
func (c *WebSocketClient) OnQueueEvent(eventType EventType, handler QueueEventHandler) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.queueEventHandlers[eventType] = append(c.queueEventHandlers[eventType], handler)
}

// OnWorkerEvent registers a handler for worker events.
func (c *WebSocketClient) OnWorkerEvent(eventType EventType, handler WorkerEventHandler) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.workerEventHandlers[eventType] = append(c.workerEventHandlers[eventType], handler)
}

// OnStateChange registers a handler for state changes.
func (c *WebSocketClient) OnStateChange(handler StateChangeHandler) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.stateChangeHandlers = append(c.stateChangeHandlers, handler)
}

func (c *WebSocketClient) readLoop() {
	defer func() {
		c.mu.Lock()
		if c.done != nil {
			close(c.done)
		}
		c.mu.Unlock()
	}()

	for {
		c.mu.RLock()
		conn := c.conn
		ctx := c.ctx
		c.mu.RUnlock()

		if conn == nil {
			return
		}

		_, data, err := conn.Read(ctx)
		if err != nil {
			c.log("WebSocket read error: %v", err)
			c.handleDisconnect()
			return
		}

		c.handleMessage(data)
	}
}

func (c *WebSocketClient) handleMessage(data []byte) {
	c.log("Received message: %s", string(data))

	// Try to parse as command response first
	var resp wsResponse
	if err := json.Unmarshal(data, &resp); err == nil {
		if resp.Type == "subscribed" || resp.Type == "unsubscribed" || resp.Type == "error" {
			c.handleCommandResponse(resp)
			return
		}
	}

	// Parse as event
	var event Event
	if err := json.Unmarshal(data, &event); err != nil {
		c.log("Failed to parse event: %v", err)
		return
	}

	c.dispatchEvent(&event)
}

func (c *WebSocketClient) handleCommandResponse(resp wsResponse) {
	if resp.RequestID == "" {
		return
	}

	c.cmdMu.Lock()
	ch, ok := c.pendingCommands[resp.RequestID]
	c.cmdMu.Unlock()

	if !ok {
		return
	}

	if resp.Type == "error" {
		ch <- fmt.Errorf("command error: %s", resp.Error)
	} else {
		ch <- nil
	}
}

func (c *WebSocketClient) dispatchEvent(event *Event) {
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

func (c *WebSocketClient) handleDisconnect() {
	c.mu.Lock()
	if c.state == StateDisconnected {
		c.mu.Unlock()
		return
	}

	c.conn = nil
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

func (c *WebSocketClient) setState(state ConnectionState) {
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

func (c *WebSocketClient) log(format string, args ...any) {
	if c.opts.Logger != nil {
		c.opts.Logger(format, args...)
	} else if c.opts.Debug {
		fmt.Printf("[spooled-ws] "+format+"\n", args...)
	}
}

func subscriptionKey(filter SubscriptionFilter) string {
	return fmt.Sprintf("%s:%s:%s", filter.QueueName, filter.JobID, filter.WorkerID)
}

func isJobEvent(t EventType) bool {
	switch t {
	case EventJobCreated, EventJobStarted, EventJobCompleted, EventJobFailed, EventJobRetrying, EventJobProgress:
		return true
	}
	return false
}

func isQueueEvent(t EventType) bool {
	switch t {
	case EventQueuePaused, EventQueueResumed:
		return true
	}
	return false
}

func isWorkerEvent(t EventType) bool {
	switch t {
	case EventWorkerJoined, EventWorkerLeft, EventWorkerActive, EventWorkerInactive:
		return true
	}
	return false
}


