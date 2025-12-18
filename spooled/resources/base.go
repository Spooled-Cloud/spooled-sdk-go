// Package resources provides REST resource implementations for the Spooled API.
package resources

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

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
func decodeResponse(resp *httpx.Response, result any) error {
	if result == nil {
		return nil
	}
	decoded, err := httpx.JSON[any](resp)
	if err != nil {
		return err
	}
	// We need to re-unmarshal into the correct type
	// This is a bit inefficient but works for all types
	return remarshal(decoded, result)
}

// remarshal re-marshals a value into a target type.
func remarshal(src, dst any) error {
	if src == nil {
		return nil
	}
	// Use JSON round-trip to convert types
	data, err := json.Marshal(src)
	if err != nil {
		return fmt.Errorf("failed to marshal: %w", err)
	}
	return json.Unmarshal(data, dst)
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
