// Package resources provides REST resource implementations for the Spooled API.
package resources

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"

	"github.com/spooled-cloud/spooled-sdk-go/internal/httpx"
)

// Base provides common functionality for all resources.
type Base struct {
	transport *httpx.Transport
}

// NewBase creates a new Base resource.
func NewBase(transport *httpx.Transport) *Base {
	return &Base{transport: transport}
}

// Get performs a GET request.
func (b *Base) Get(ctx context.Context, path string, result any) error {
	resp, err := b.transport.Do(ctx, &httpx.Request{
		Method: http.MethodGet,
		Path:   path,
	})
	if err != nil {
		return err
	}
	return decodeResponse(resp, result)
}

// GetWithQuery performs a GET request with query parameters.
func (b *Base) GetWithQuery(ctx context.Context, path string, query url.Values, result any) error {
	resp, err := b.transport.Do(ctx, &httpx.Request{
		Method: http.MethodGet,
		Path:   path,
		Query:  valuestoMap(query),
	})
	if err != nil {
		return err
	}
	return decodeResponse(resp, result)
}

// hasMoreHeader is the response header with which the list endpoints say
// whether another page follows (their bodies are bare arrays).
const hasMoreHeader = "X-Has-More"

// getPage performs a GET request and also returns the X-Has-More paging hint:
// true only when the API sent "true". An API that sends no header reports
// false, so callers fall back to comparing the page length with the limit.
func (b *Base) getPage(ctx context.Context, path string, query url.Values, result any) (bool, error) {
	resp, err := b.transport.Do(ctx, &httpx.Request{
		Method: http.MethodGet,
		Path:   path,
		Query:  valuestoMap(query),
	})
	if err != nil {
		return false, err
	}
	if err := decodeResponse(resp, result); err != nil {
		return false, err
	}
	hasMore, _ := strconv.ParseBool(strings.TrimSpace(resp.Headers.Get(hasMoreHeader)))
	return hasMore, nil
}

// Post performs a POST request.
func (b *Base) Post(ctx context.Context, path string, body any, result any) error {
	resp, err := b.transport.Do(ctx, &httpx.Request{
		Method: http.MethodPost,
		Path:   path,
		Body:   body,
	})
	if err != nil {
		return err
	}
	return decodeResponse(resp, result)
}

// PostIdempotent performs an idempotent POST request (can be retried).
func (b *Base) PostIdempotent(ctx context.Context, path string, body any, result any) error {
	resp, err := b.transport.Do(ctx, &httpx.Request{
		Method:     http.MethodPost,
		Path:       path,
		Body:       body,
		Idempotent: true,
	})
	if err != nil {
		return err
	}
	return decodeResponse(resp, result)
}

// PostAck performs a worker acknowledgement POST (complete, fail, heartbeat).
//
// Acks are lease-fenced, so repeating one is harmless: a duplicate is answered
// with 409 and never applied twice. They are therefore retried on network
// errors, timeouts, 408, 429 and 5xx like an idempotent request, and they ignore
// an open circuit breaker. Losing an ack throws away finished work and leaves
// the job "processing" until its lease expires.
func (b *Base) PostAck(ctx context.Context, path string, body any, result any) error {
	resp, err := b.transport.Do(ctx, &httpx.Request{
		Method:             http.MethodPost,
		Path:               path,
		Body:               body,
		Idempotent:         true,
		SkipCircuitBreaker: true,
	})
	if err != nil {
		return err
	}
	return decodeResponse(resp, result)
}

// Put performs a PUT request.
func (b *Base) Put(ctx context.Context, path string, body any, result any) error {
	resp, err := b.transport.Do(ctx, &httpx.Request{
		Method: http.MethodPut,
		Path:   path,
		Body:   body,
	})
	if err != nil {
		return err
	}
	return decodeResponse(resp, result)
}

// Patch performs a PATCH request.
func (b *Base) Patch(ctx context.Context, path string, body any, result any) error {
	resp, err := b.transport.Do(ctx, &httpx.Request{
		Method: http.MethodPatch,
		Path:   path,
		Body:   body,
	})
	if err != nil {
		return err
	}
	return decodeResponse(resp, result)
}

// Delete performs a DELETE request.
func (b *Base) Delete(ctx context.Context, path string) error {
	_, err := b.transport.Do(ctx, &httpx.Request{
		Method: http.MethodDelete,
		Path:   path,
	})
	return err
}

// DeleteWithBody performs a DELETE request with a body.
func (b *Base) DeleteWithBody(ctx context.Context, path string, body any, result any) error {
	resp, err := b.transport.Do(ctx, &httpx.Request{
		Method: http.MethodDelete,
		Path:   path,
		Body:   body,
	})
	if err != nil {
		return err
	}
	return decodeResponse(resp, result)
}

// AdminGet performs a GET request with admin key.
func (b *Base) AdminGet(ctx context.Context, path string, result any) error {
	resp, err := b.transport.Do(ctx, &httpx.Request{
		Method:      http.MethodGet,
		Path:        path,
		UseAdminKey: true,
	})
	if err != nil {
		return err
	}
	return decodeResponse(resp, result)
}

// AdminPost performs a POST request with admin key.
func (b *Base) AdminPost(ctx context.Context, path string, body any, result any) error {
	resp, err := b.transport.Do(ctx, &httpx.Request{
		Method:      http.MethodPost,
		Path:        path,
		Body:        body,
		UseAdminKey: true,
	})
	if err != nil {
		return err
	}
	return decodeResponse(resp, result)
}

// AdminPut performs a PUT request with admin key.
func (b *Base) AdminPut(ctx context.Context, path string, body any, result any) error {
	resp, err := b.transport.Do(ctx, &httpx.Request{
		Method:      http.MethodPut,
		Path:        path,
		Body:        body,
		UseAdminKey: true,
	})
	if err != nil {
		return err
	}
	return decodeResponse(resp, result)
}

// AdminPatch performs a PATCH request with admin key.
func (b *Base) AdminPatch(ctx context.Context, path string, body any, result any) error {
	resp, err := b.transport.Do(ctx, &httpx.Request{
		Method:      http.MethodPatch,
		Path:        path,
		Body:        body,
		UseAdminKey: true,
	})
	if err != nil {
		return err
	}
	return decodeResponse(resp, result)
}

// AdminDelete performs a DELETE request with admin key.
func (b *Base) AdminDelete(ctx context.Context, path string) error {
	_, err := b.transport.Do(ctx, &httpx.Request{
		Method:      http.MethodDelete,
		Path:        path,
		UseAdminKey: true,
	})
	return err
}

// decodeResponse decodes a response into the result if result is not nil.
//
// The body goes through a normalised intermediate tree first. That keeps the
// historical leniency (a whole-number float such as 5.0 decodes into an int
// field) while numbers that float64 cannot hold exactly - bigint IDs in job
// payloads - survive as json.Number instead of being rounded. See numbers.go.
func decodeResponse(resp *httpx.Response, result any) error {
	if result == nil || len(resp.Body) == 0 {
		return nil
	}
	var tree any
	if err := unmarshalJSON(resp.Body, &tree); err != nil {
		return fmt.Errorf("failed to unmarshal response: %w", err)
	}
	data, err := json.Marshal(normalizeTree(tree))
	if err != nil {
		return fmt.Errorf("failed to marshal: %w", err)
	}
	if err := unmarshalJSON(data, result); err != nil {
		return err
	}
	restoreNumbers(reflect.ValueOf(result))
	return nil
}

// valuestoMap converts url.Values to map[string]string (taking first value).
func valuestoMap(v url.Values) map[string]string {
	if v == nil {
		return nil
	}
	m := make(map[string]string, len(v))
	for k := range v {
		if val := v.Get(k); val != "" {
			m[k] = val
		}
	}
	return m
}

// Pagination helpers

// AddPaginationParams adds pagination parameters to query values.
func AddPaginationParams(v url.Values, limit, offset *int) {
	if limit != nil {
		v.Set("limit", strconv.Itoa(*limit))
	}
	if offset != nil {
		v.Set("offset", strconv.Itoa(*offset))
	}
}

// paginate applies offset and limit to a list that was filtered client-side.
// Negative values are treated as unset.
func paginate[T any](items []T, offset, limit *int) []T {
	if offset != nil && *offset > 0 {
		if *offset >= len(items) {
			return []T{}
		}
		items = items[*offset:]
	}
	if limit != nil && *limit >= 0 && *limit < len(items) {
		items = items[:*limit]
	}
	return items
}
