// Example: Quick Start with Spooled Go SDK
//
// This example demonstrates basic job creation and retrieval.
//
// Usage:
//
//	API_KEY=sp_test_... go run main.go
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

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
	queueName := fmt.Sprintf("example-%d", time.Now().Unix())

	// Create a job
	fmt.Println("Creating job...")
	priority := 5
	maxRetries := 3
	result, err := client.Jobs().Create(ctx, &resources.CreateJobRequest{
		QueueName: queueName,
		Payload: map[string]any{
			"to":      "user@example.com",
			"subject": "Welcome!",
			"body":    "Thanks for signing up.",
		},
		Priority:   &priority,
		MaxRetries: &maxRetries,
	})
	if err != nil {
		log.Fatalf("Failed to create job: %v", err)
	}
	fmt.Printf("✓ Created job: %s\n", result.ID)

	// Get job status
	fmt.Println("\nGetting job status...")
	job, err := client.Jobs().Get(ctx, result.ID)
	if err != nil {
		log.Fatalf("Failed to get job: %v", err)
	}
	fmt.Printf("✓ Job ID: %s\n", job.ID)
	fmt.Printf("  Queue: %s\n", job.QueueName)
	fmt.Printf("  Status: %s\n", job.Status)
	fmt.Printf("  Priority: %d\n", job.Priority)

	// List jobs in queue
	fmt.Println("\nListing jobs in queue...")
	limit := 10
	jobs, err := client.Jobs().List(ctx, &resources.ListJobsParams{
		QueueName: &queueName,
		Limit:     &limit,
	})
	if err != nil {
		log.Fatalf("Failed to list jobs: %v", err)
	}
	fmt.Printf("✓ Found %d job(s) in queue\n", len(jobs))

	// Cancel the job
	fmt.Println("\nCancelling job...")
	if err := client.Jobs().Cancel(ctx, result.ID); err != nil {
		log.Fatalf("Failed to cancel job: %v", err)
	}
	fmt.Printf("✓ Job cancelled\n")

	// Verify cancellation
	job, err = client.Jobs().Get(ctx, result.ID)
	if err != nil {
		log.Fatalf("Failed to get job: %v", err)
	}
	fmt.Printf("  Final status: %s\n", job.Status)

	fmt.Println("\n✓ Quick start example complete!")
}


