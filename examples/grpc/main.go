// Example: gRPC High-Performance Client
//
// This example demonstrates using the gRPC client for high-throughput scenarios.
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

	spooledgrpc "github.com/spooled-cloud/spooled-sdk-go/spooled/grpc"
)

func main() {
	apiKey := os.Getenv("API_KEY")
	if apiKey == "" {
		log.Fatal("API_KEY environment variable is required")
	}

	grpcAddress := os.Getenv("GRPC_ADDRESS")
	if grpcAddress == "" {
		grpcAddress = "grpc.spooled.cloud:443"
	}

	queueName := fmt.Sprintf("grpc-example-%d", time.Now().Unix())

	// Create gRPC client
	fmt.Printf("Connecting to gRPC server: %s\n", grpcAddress)
	client, err := spooledgrpc.NewClient(spooledgrpc.ClientOptions{
		Address: grpcAddress,
		APIKey:  apiKey,
	})
	if err != nil {
		log.Fatalf("Failed to create gRPC client: %v", err)
	}
	defer client.Close()
	fmt.Println("✓ Connected")

	ctx := context.Background()

	// Enqueue jobs
	fmt.Println("\nEnqueuing jobs via gRPC...")
	for i := 0; i < 5; i++ {
		resp, err := client.Enqueue(ctx, &spooledgrpc.EnqueueRequest{
			QueueName: queueName,
			Payload: map[string]any{
				"index":     i,
				"timestamp": time.Now().Format(time.RFC3339),
			},
			Priority:   5,
			MaxRetries: 3,
		})
		if err != nil {
			log.Printf("Failed to enqueue job %d: %v", i, err)
			continue
		}
		fmt.Printf("  ✓ Enqueued job %d: %s\n", i, resp.JobID)
	}

	// Get queue stats
	fmt.Println("\nGetting queue stats...")
	stats, err := client.GetQueueStats(ctx, queueName)
	if err != nil {
		log.Printf("Failed to get stats: %v", err)
	} else {
		fmt.Printf("  Queue: %s\n", queueName)
		fmt.Printf("  Pending: %d\n", stats.Pending)
		fmt.Printf("  Processing: %d\n", stats.Processing)
	}

	// Dequeue jobs
	fmt.Println("\nDequeuing jobs...")
	workerID := fmt.Sprintf("worker-%d", time.Now().Unix())
	resp, err := client.Dequeue(ctx, &spooledgrpc.DequeueRequest{
		QueueName: queueName,
		WorkerID:  workerID,
		BatchSize: 3,
	})
	if err != nil {
		log.Printf("Failed to dequeue: %v", err)
	} else {
		fmt.Printf("  ✓ Dequeued %d job(s)\n", len(resp.Jobs))
		for _, job := range resp.Jobs {
			fmt.Printf("    - %s (priority: %d)\n", job.ID, job.Priority)

			// Complete the job
			err := client.Complete(ctx, &spooledgrpc.CompleteRequest{
				JobID:    job.ID,
				WorkerID: workerID,
				Result: map[string]any{
					"processed": true,
				},
			})
			if err != nil {
				log.Printf("    Failed to complete: %v", err)
			} else {
				fmt.Printf("    ✓ Completed\n")
			}
		}
	}

	fmt.Println("\n✓ gRPC example complete!")
}
