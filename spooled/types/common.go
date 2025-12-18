// Package types contains request and response types for the Spooled API.
package types

import "time"

// JsonObject is a generic JSON object.
type JsonObject = map[string]any

// Pagination parameters for list operations.
type PaginationParams struct {
	Limit    *int    `json:"limit,omitempty"`
	Offset   *int    `json:"offset,omitempty"`
	OrderBy  *string `json:"order_by,omitempty"`
	OrderDir *string `json:"order_dir,omitempty"` // "asc" or "desc"
}

// Int returns a pointer to an int.
func Int(v int) *int {
	return &v
}

// String returns a pointer to a string.
func String(v string) *string {
	return &v
}

// Bool returns a pointer to a bool.
func Bool(v bool) *bool {
	return &v
}

// Time returns a pointer to a time.Time.
func Time(v time.Time) *time.Time {
	return &v
}
