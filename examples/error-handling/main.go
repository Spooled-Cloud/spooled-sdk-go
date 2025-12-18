// Example: Error Handling
//
// This example demonstrates proper error handling with the SDK.
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

	"github.com/spooled-cloud/spooled-sdk-go/internal/httpx"
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

	ctx := context.Background()

	fmt.Println("Error Handling Examples")
	fmt.Println("=======================")

	// Example 1: Not Found Error
	fmt.Println("\n1. Handling Not Found errors:")
	job, err := client.Jobs().Get(ctx, "non-existent-job-id")
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
		spooled.WithAPIKey("invalid-api-key"),
		spooled.WithBaseURL(baseURL),
	)
	_, err = badClient.Health().Get(ctx)
	if err != nil {
		handleError("Request with invalid API key", err)
	}

	// Example 4: Demonstrating error type checks
	fmt.Println("\n4. Using error type checks:")
	demoErrorChecks()

	fmt.Println("\n✓ Error handling example complete!")
}

func handleError(operation string, err error) {
	fmt.Printf("  %s:\n", operation)

	// Check specific error types using errors.As
	var apiErr *httpx.APIError
	var circuitErr *httpx.CircuitBreakerOpenError
	var timeoutErr *httpx.TimeoutError
	var networkErr *httpx.NetworkError

	switch {
	case errors.As(err, &apiErr):
		fmt.Printf("    Type: APIError\n")
		fmt.Printf("    Status: %d\n", apiErr.StatusCode)
		fmt.Printf("    Code: %s\n", apiErr.Code)
		fmt.Printf("    Message: %s\n", apiErr.Message)

		// Check for specific error codes
		switch apiErr.Code {
		case "NOT_FOUND":
			fmt.Printf("    Action: Resource doesn't exist - check ID\n")
		case "VALIDATION_ERROR":
			fmt.Printf("    Action: Fix the request parameters\n")
		case "UNAUTHORIZED", "FORBIDDEN":
			fmt.Printf("    Action: Check API key or permissions\n")
		case "RATE_LIMIT_EXCEEDED":
			fmt.Printf("    Action: Wait and retry\n")
		default:
			fmt.Printf("    Action: Check request and retry\n")
		}

	case errors.As(err, &circuitErr):
		fmt.Printf("    Type: CircuitBreakerOpenError\n")
		fmt.Printf("    Action: Service is unhealthy - wait for recovery\n")

	case errors.As(err, &timeoutErr):
		fmt.Printf("    Type: TimeoutError\n")
		fmt.Printf("    Action: Retry or increase timeout\n")

	case errors.As(err, &networkErr):
		fmt.Printf("    Type: NetworkError\n")
		fmt.Printf("    Action: Check network connectivity\n")

	default:
		fmt.Printf("    Type: Unknown (%T)\n", err)
		fmt.Printf("    Error: %v\n", err)
	}
}

func demoErrorChecks() {
	// Demonstrate checking if errors are retryable
	testCases := []struct {
		name string
		err  error
	}{
		{"Network error", httpx.NewNetworkError(fmt.Errorf("connection refused"))},
		{"Timeout error", httpx.NewTimeoutError(30, context.DeadlineExceeded)},
		{"Circuit breaker", &httpx.CircuitBreakerOpenError{}},
		{"API 404", &httpx.APIError{StatusCode: 404, Code: "NOT_FOUND", Message: "Resource not found"}},
		{"API 500", &httpx.APIError{StatusCode: 500, Code: "INTERNAL_ERROR", Message: "Server error"}},
	}

	for _, tc := range testCases {
		fmt.Printf("  %s:\n", tc.name)
		fmt.Printf("    IsRetryable: %v\n", httpx.IsRetryable(tc.err))
		fmt.Printf("    Error: %v\n", tc.err)
	}
}
