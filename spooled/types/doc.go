// Package types provides request and response types for the Spooled API.
//
// All types in this package are designed to be serialized to/from JSON with
// snake_case field names as expected by the Spooled API.
//
// # Optional Fields
//
// Optional fields are represented as pointers. Helper functions are provided
// to create pointers to values:
//
//	req := &types.CreateJobRequest{
//		QueueName: "emails",
//		Payload:   payload,
//		Priority:  types.Int(100),     // optional priority
//		MaxRetries: types.Int(5),      // optional max retries
//	}
//
// # Enums
//
// Enums are represented as string type aliases with constants for known values:
//
//	status := types.JobStatusPending
//
// Unknown values returned by the API will be preserved and can be compared
// as strings.
package types
