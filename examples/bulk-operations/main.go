// Example: Bulk Operations
//
// This example demonstrates bulk job operations for high throughput.
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
	queueName := fmt.Sprintf("bulk-example-%d", time.Now().Unix())

	// Bulk enqueue jobs
	fmt.Println("Bulk enqueuing 100 jobs...")
	start := time.Now()

	jobs := make([]resources.BulkJobItem, 100)
	for i := 0; i < 100; i++ {
		priority := i % 10 // Varying priorities 0-9
		jobs[i] = resources.BulkJobItem{
			Payload: map[string]any{
				"index":     i,
				"batch":     "example",
				"timestamp": time.Now().Format(time.RFC3339),
			},
			Priority: &priority,
		}
	}

	result, err := client.Jobs().BulkEnqueue(ctx, &resources.BulkEnqueueRequest{
		QueueName: queueName,
		Jobs:      jobs,
	})
	if err != nil {
		log.Fatalf("Failed to bulk enqueue: %v", err)
	}

	duration := time.Since(start)
	fmt.Printf("✓ Enqueued %d jobs in %v\n", result.SuccessCount, duration)
	fmt.Printf("  Success: %d, Failed: %d\n", result.SuccessCount, result.FailureCount)
	fmt.Printf("  Rate: %.0f jobs/second\n", float64(result.SuccessCount)/duration.Seconds())

	// Get queue stats
	fmt.Println("\nGetting queue stats...")
	stats, err := client.Queues().GetStats(ctx, queueName)
	if err != nil {
		log.Printf("Failed to get stats: %v", err)
	} else {
		fmt.Printf("✓ Queue: %s\n", queueName)
		fmt.Printf("  Pending: %d\n", stats.PendingJobs)
		fmt.Printf("  Processing: %d\n", stats.ProcessingJobs)
	}

	// Get batch status for first 10 jobs
	fmt.Println("\nGetting batch status for first 10 jobs...")
	jobIDs := make([]string, 0, 10)
	for i := 0; i < 10 && i < len(result.Succeeded); i++ {
		jobIDs = append(jobIDs, result.Succeeded[i].JobID)
	}

	if len(jobIDs) > 0 {
		statuses, err := client.Jobs().BatchStatus(ctx, jobIDs)
		if err != nil {
			log.Printf("Failed to get batch status: %v", err)
		} else {
			fmt.Printf("✓ Got status for %d jobs\n", len(statuses))
			statusCounts := make(map[string]int)
			for _, job := range statuses {
				statusCounts[string(job.Status)]++
			}
			for status, count := range statusCounts {
				fmt.Printf("  %s: %d\n", status, count)
			}
		}
	}

	// List jobs with pagination
	fmt.Println("\nListing jobs with pagination...")
	limit := 25
	offset := 0
	totalListed := 0

	for {
		jobs, err := client.Jobs().List(ctx, &resources.ListJobsParams{
			QueueName: &queueName,
			Limit:     &limit,
			Offset:    &offset,
		})
		if err != nil {
			log.Printf("Failed to list jobs: %v", err)
			break
		}

		totalListed += len(jobs)
		fmt.Printf("  Page %d: %d jobs\n", offset/limit+1, len(jobs))

		if len(jobs) < limit {
			break // No more pages
		}
		offset += limit

		if offset >= 100 {
			break // Just show first 4 pages for example
		}
	}
	fmt.Printf("✓ Listed %d jobs total\n", totalListed)

	// Cancel all jobs in queue
	fmt.Println("\nCancelling all jobs...")
	cancelled := 0
	for _, job := range result.Succeeded {
		if err := client.Jobs().Cancel(ctx, job.JobID); err == nil {
			cancelled++
		}
	}
	fmt.Printf("✓ Cancelled %d jobs\n", cancelled)

	fmt.Println("\n✓ Bulk operations example complete!")
}
