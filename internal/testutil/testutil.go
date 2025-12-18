// Package testutil provides testing utilities for the Spooled SDK.
package testutil

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// MockResponse represents a mock HTTP response.
type MockResponse struct {
	StatusCode int
	Body       any
	Headers    map[string]string
}

// MockServer is a test HTTP server for mocking API responses.
type MockServer struct {
	*httptest.Server
	mu       sync.Mutex
	handlers map[string]map[string]http.HandlerFunc
	requests []RecordedRequest
}

// RecordedRequest represents a recorded HTTP request.
type RecordedRequest struct {
	Method  string
	Path    string
	Headers http.Header
	Body    []byte
}

// NewMockServer creates a new mock server.
func NewMockServer(t *testing.T) *MockServer {
	t.Helper()

	ms := &MockServer{
		handlers: make(map[string]map[string]http.HandlerFunc),
		requests: make([]RecordedRequest, 0),
	}

	ms.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Record request
		body, _ := io.ReadAll(r.Body)
		r.Body.Close()

		ms.mu.Lock()
		ms.requests = append(ms.requests, RecordedRequest{
			Method:  r.Method,
			Path:    r.URL.Path,
			Headers: r.Header.Clone(),
			Body:    body,
		})
		ms.mu.Unlock()

		// Find handler
		if methodHandlers, ok := ms.handlers[r.URL.Path]; ok {
			if handler, ok := methodHandlers[r.Method]; ok {
				// Create new request with body for handler
				newBody := io.NopCloser(strings.NewReader(string(body)))
				r.Body = newBody
				handler(w, r)
				return
			}
		}

		// Default 404
		http.Error(w, "not found", http.StatusNotFound)
	}))

	t.Cleanup(func() {
		ms.Close()
	})

	return ms
}

// Handle registers a handler for a specific method and path.
func (ms *MockServer) Handle(method, path string, handler http.HandlerFunc) {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	if ms.handlers[path] == nil {
		ms.handlers[path] = make(map[string]http.HandlerFunc)
	}
	ms.handlers[path][method] = handler
}

// HandleJSON registers a handler that returns a JSON response.
func (ms *MockServer) HandleJSON(method, path string, statusCode int, response any) {
	ms.Handle(method, path, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		if response != nil {
			json.NewEncoder(w).Encode(response)
		}
	})
}

// HandleError registers a handler that returns an error response.
func (ms *MockServer) HandleError(method, path string, statusCode int, code, message string) {
	ms.HandleJSON(method, path, statusCode, map[string]any{
		"code":    code,
		"message": message,
	})
}

// GetRequests returns all recorded requests.
func (ms *MockServer) GetRequests() []RecordedRequest {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	return append([]RecordedRequest{}, ms.requests...)
}

// LastRequest returns the last recorded request.
func (ms *MockServer) LastRequest() *RecordedRequest {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	if len(ms.requests) == 0 {
		return nil
	}
	return &ms.requests[len(ms.requests)-1]
}

// ClearRequests clears all recorded requests.
func (ms *MockServer) ClearRequests() {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	ms.requests = make([]RecordedRequest, 0)
}

// AssertRequestCount asserts that a specific number of requests were made.
func (ms *MockServer) AssertRequestCount(t *testing.T, expected int) {
	t.Helper()
	ms.mu.Lock()
	actual := len(ms.requests)
	ms.mu.Unlock()

	if actual != expected {
		t.Errorf("expected %d requests, got %d", expected, actual)
	}
}

// AssertLastRequestMethod asserts the method of the last request.
func (ms *MockServer) AssertLastRequestMethod(t *testing.T, expected string) {
	t.Helper()
	req := ms.LastRequest()
	if req == nil {
		t.Error("no requests recorded")
		return
	}
	if req.Method != expected {
		t.Errorf("expected method %s, got %s", expected, req.Method)
	}
}

// AssertLastRequestPath asserts the path of the last request.
func (ms *MockServer) AssertLastRequestPath(t *testing.T, expected string) {
	t.Helper()
	req := ms.LastRequest()
	if req == nil {
		t.Error("no requests recorded")
		return
	}
	if req.Path != expected {
		t.Errorf("expected path %s, got %s", expected, req.Path)
	}
}

// AssertLastRequestHeader asserts a header of the last request.
func (ms *MockServer) AssertLastRequestHeader(t *testing.T, key, expected string) {
	t.Helper()
	req := ms.LastRequest()
	if req == nil {
		t.Error("no requests recorded")
		return
	}
	actual := req.Headers.Get(key)
	if actual != expected {
		t.Errorf("expected header %s=%s, got %s", key, expected, actual)
	}
}

// ParseLastRequestBody parses the body of the last request as JSON.
func (ms *MockServer) ParseLastRequestBody(t *testing.T, v any) {
	t.Helper()
	req := ms.LastRequest()
	if req == nil {
		t.Error("no requests recorded")
		return
	}
	if err := json.Unmarshal(req.Body, v); err != nil {
		t.Errorf("failed to parse request body: %v", err)
	}
}
