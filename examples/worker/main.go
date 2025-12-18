// Example: Worker Runtime
//
// This example demonstrates processing jobs with the worker runtime.
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
	"os/signal"
	"syscall"
	"time"

	"github.com/spooled-cloud/spooled-sdk-go/spooled"
	"github.com/spooled-cloud/spooled-sdk-go/spooled/resources"
	"github.com/spooled-cloud/spooled-sdk-go/spooled/worker"
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

	queueName := fmt.Sprintf("worker-example-%d", time.Now().Unix())
	ctx := context.Background()

	// Create some test jobs
	fmt.Println("Creating test jobs...")
	for i := 0; i < 5; i++ {
		_, err := client.Jobs().Create(ctx, &resources.CreateJobRequest{
			QueueName: queueName,
			Payload: map[string]any{
				"task_id": i,
				"message": fmt.Sprintf("Task %d", i),
			},
		})
		if err != nil {
			log.Fatalf("Failed to create job: %v", err)
		}
	}
	fmt.Printf("✓ Created 5 test jobs in queue: %s\n\n", queueName)

	// Create worker
	w := worker.NewWorker(client.Jobs(), client.Workers(), worker.Options{
		QueueName:   queueName,
		Concurrency: 3,
		Debug:       true,
	})

	// Register job handler
	w.Process(func(ctx *worker.JobContext) (map[string]any, error) {
		fmt.Printf("  Processing job %s...\n", ctx.JobID)

		// Simulate work
		taskID := ctx.Payload["task_id"]
		message := ctx.Payload["message"]
		fmt.Printf("    Task ID: %v, Message: %v\n", taskID, message)

		// Report progress
		ctx.Progress(50, "Processing...")
		time.Sleep(500 * time.Millisecond)

		ctx.Progress(100, "Complete")

		return map[string]any{
			"processed": true,
			"task_id":   taskID,
		}, nil
	})

	// Event handlers
	w.OnEvent(func(event worker.Event) {
		switch event.Type {
		case worker.EventWorkerStarted:
			data := event.Data.(worker.WorkerStartedData)
			fmt.Printf("Worker started: %s\n", data.WorkerID)
		case worker.EventJobCompleted:
			data := event.Data.(worker.JobCompletedData)
			fmt.Printf("✓ Job completed: %s (duration: %v)\n", data.JobID, data.Duration)
		case worker.EventJobFailed:
			data := event.Data.(worker.JobFailedData)
			fmt.Printf("✗ Job failed: %s (error: %v)\n", data.JobID, data.Error)
		case worker.EventWorkerStopped:
			fmt.Println("Worker stopped")
		}
	})

	// Start worker
	workerCtx, cancel := context.WithCancel(context.Background())
	fmt.Println("Starting worker...")
	if err := w.Start(workerCtx); err != nil {
		log.Fatalf("Failed to start worker: %v", err)
	}

	// Wait for signal or timeout
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Auto-stop after 30 seconds for the example
	go func() {
		time.Sleep(30 * time.Second)
		fmt.Println("\nAuto-stopping after 30 seconds...")
		sigCh <- syscall.SIGTERM
	}()

	<-sigCh
	fmt.Println("\nShutting down...")

	cancel()
	w.Stop()

	fmt.Println("\n✓ Worker example complete!")
}
