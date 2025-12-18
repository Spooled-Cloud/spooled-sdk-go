// Example: Cron Schedules
//
// This example demonstrates creating and managing cron schedules.
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

func ptr[T any](v T) *T {
	return &v
}

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
	queueName := fmt.Sprintf("schedules-example-%d", time.Now().Unix())

	// Create a schedule that runs every minute
	fmt.Println("Creating schedule...")
	schedule, err := client.Schedules().Create(ctx, &resources.CreateScheduleRequest{
		Name:           fmt.Sprintf("Example Schedule %d", time.Now().Unix()),
		CronExpression: "* * * * *", // Every minute
		Timezone:       ptr("America/New_York"),
		QueueName:      queueName,
		PayloadTemplate: map[string]any{
			"type":       "scheduled-task",
			"created_at": time.Now().Format(time.RFC3339),
		},
	})
	if err != nil {
		log.Fatalf("Failed to create schedule: %v", err)
	}

	fmt.Printf("✓ Schedule created: %s\n", schedule.ID)
	fmt.Printf("  Name: %s\n", schedule.Name)
	fmt.Printf("  Cron: %s\n", schedule.CronExpression)
	fmt.Printf("  Active: %v\n", schedule.IsActive)
	if schedule.NextRunAt != nil {
		fmt.Printf("  Next run: %v\n", schedule.NextRunAt)
	}
	fmt.Println()

	// Get schedule details
	fmt.Println("Getting schedule details...")
	s, err := client.Schedules().Get(ctx, schedule.ID)
	if err != nil {
		log.Fatalf("Failed to get schedule: %v", err)
	}
	fmt.Printf("  Active: %v\n", s.IsActive)
	fmt.Printf("  Queue: %s\n", s.QueueName)
	fmt.Println()

	// Pause the schedule
	fmt.Println("Pausing schedule...")
	pausedSchedule, err := client.Schedules().Pause(ctx, schedule.ID)
	if err != nil {
		log.Fatalf("Failed to pause schedule: %v", err)
	}
	fmt.Printf("✓ Schedule paused, active: %v\n\n", pausedSchedule.IsActive)

	// Resume the schedule
	fmt.Println("Resuming schedule...")
	resumedSchedule, err := client.Schedules().Resume(ctx, schedule.ID)
	if err != nil {
		log.Fatalf("Failed to resume schedule: %v", err)
	}
	fmt.Printf("✓ Schedule resumed, active: %v\n\n", resumedSchedule.IsActive)

	// Trigger immediately
	fmt.Println("Triggering schedule immediately...")
	triggered, err := client.Schedules().Trigger(ctx, schedule.ID)
	if err != nil {
		log.Fatalf("Failed to trigger schedule: %v", err)
	}
	fmt.Printf("✓ Schedule triggered, job ID: %s\n\n", triggered.JobID)

	// List schedules
	fmt.Println("Listing schedules...")
	limit := 10
	schedules, err := client.Schedules().List(ctx, &resources.ListSchedulesParams{
		Limit: &limit,
	})
	if err != nil {
		log.Fatalf("Failed to list schedules: %v", err)
	}
	fmt.Printf("✓ Found %d schedule(s)\n\n", len(schedules))

	// Delete the schedule
	fmt.Println("Deleting schedule...")
	if err := client.Schedules().Delete(ctx, schedule.ID); err != nil {
		log.Fatalf("Failed to delete schedule: %v", err)
	}
	fmt.Printf("✓ Schedule deleted\n")

	fmt.Println("\n✓ Schedules example complete!")
}
