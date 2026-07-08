// Example: Error Handling
//
// This example demonstrates proper error handling with the SDK using only the
// public spooled package: the spooled.Is*Error classifiers, spooled.AsSpooledError
// for structured access, and spooled.IsRetryable.
//
// Usage:
//
//	API_KEY=sp_test_... go run main.go
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"

	"github.com/spooled-cloud/spooled-sdk-go/spooled"
	"github.com/spooled-cloud/spooled-sdk-go/spooled/resources"
)

func main() {
	apiKey := os.Getenv("API_KEY")
	if apiKey == "" {
		log.Fatal("API_KEY environment variable is required")
	}

	baseURL := os.Getenv("BASE_URL")
	if baseURL == "" {
		baseURL = "https://api.spooled.cloud"
	}

	// Create client
	client, err := spooled.NewClient(
		spooled.WithAPIKey(apiKey),
		spooled.WithBaseURL(baseURL),
	)
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	ctx := context.Background()

	fmt.Println("Error Handling Examples")
	fmt.Println("=======================")

	// Example 1: Not Found Error
	fmt.Println("\n1. Handling Not Found errors:")
	job, err := client.Jobs().Get(ctx, "00000000-0000-0000-0000-000000000000")
	if err != nil {
		handleError("Get non-existent job", err)
	} else {
		fmt.Printf("  Unexpected success: %s\n", job.ID)
	}

	// Example 2: Validation Error
	fmt.Println("\n2. Handling Validation errors:")
	_, err = client.Jobs().Create(ctx, &resources.CreateJobRequest{
		QueueName: "", // Invalid: empty queue name
		Payload:   map[string]any{"test": true},
	})
	if err != nil {
		handleError("Create job with empty queue", err)
	}

	// Example 3: Authentication Error
	fmt.Println("\n3. Handling Authentication errors:")
	badClient, _ := spooled.NewClient(
		spooled.WithAPIKey("sp_test_invalid00000000000000000000"),
		spooled.WithBaseURL(baseURL),
	)
	defer badClient.Close()
	_, err = badClient.Health().Get(ctx)
	if err != nil {
		handleError("Request with invalid API key", err)
	}

	// Example 4: Demonstrating error type checks
	fmt.Println("\n4. Using error type checks:")
	demoErrorChecks()

	fmt.Println("\nDone. Error handling example complete.")
}

// handleError classifies an error using only the public spooled API.
func handleError(operation string, err error) {
	fmt.Printf("  %s:\n", operation)

	switch {
	case spooled.IsNotFoundError(err):
		fmt.Printf("    Type: NotFoundError\n")
		fmt.Printf("    Action: Resource doesn't exist - check the ID\n")

	case spooled.IsValidationError(err):
		fmt.Printf("    Type: ValidationError\n")
		fmt.Printf("    Action: Fix the request parameters\n")

	case spooled.IsAuthenticationError(err):
		fmt.Printf("    Type: AuthenticationError\n")
		fmt.Printf("    Action: Check the API key\n")

	case spooled.IsAuthorizationError(err):
		fmt.Printf("    Type: AuthorizationError\n")
		fmt.Printf("    Action: Check account permissions\n")

	case spooled.IsRateLimitError(err):
		fmt.Printf("    Type: RateLimitError\n")
		var rateLimitErr *spooled.RateLimitError
		if errors.As(err, &rateLimitErr) {
			fmt.Printf("    Action: Wait %d seconds and retry\n", rateLimitErr.GetRetryAfter())
		}

	case spooled.IsServerError(err):
		fmt.Printf("    Type: ServerError\n")
		fmt.Printf("    Action: Transient server error - retry with backoff\n")

	default:
		fmt.Printf("    Type: Unknown (%T)\n", err)
		fmt.Printf("    Error: %v\n", err)
	}

	// AsSpooledError exposes the structured fields for any Spooled API error.
	if apiErr, ok := spooled.AsSpooledError(err); ok {
		fmt.Printf("    Status: %d\n", apiErr.StatusCode)
		fmt.Printf("    Code: %s\n", apiErr.Code)
		fmt.Printf("    Message: %s\n", apiErr.Message)
		if apiErr.RequestID != "" {
			fmt.Printf("    RequestID: %s\n", apiErr.RequestID)
		}
		fmt.Printf("    Retryable: %v\n", spooled.IsRetryable(err))
	}
}

// demoErrorChecks demonstrates spooled.IsRetryable on synthetic errors built
// entirely from the public error types.
func demoErrorChecks() {
	testCases := []struct {
		name string
		err  error
	}{
		{"Network error", &spooled.NetworkError{APIError: &spooled.APIError{Code: "network_error", Message: "connection refused"}}},
		{"Timeout error", &spooled.TimeoutError{APIError: &spooled.APIError{Code: "timeout", Message: "request timed out"}}},
		{"Circuit breaker", &spooled.CircuitBreakerOpenError{APIError: &spooled.APIError{Code: "circuit_breaker_open", Message: "circuit breaker is open"}}},
		{"API 404", &spooled.APIError{StatusCode: 404, Code: "NOT_FOUND", Message: "Resource not found"}},
		{"API 500", &spooled.APIError{StatusCode: 500, Code: "INTERNAL_ERROR", Message: "Server error"}},
	}

	for _, tc := range testCases {
		fmt.Printf("  %s:\n", tc.name)
		fmt.Printf("    IsRetryable: %v\n", spooled.IsRetryable(tc.err))
		fmt.Printf("    IsSpooledError: %v\n", spooled.IsSpooledError(tc.err))
		fmt.Printf("    Error: %v\n", tc.err)
	}
}
