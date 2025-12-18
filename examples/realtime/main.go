// Example: Real-time Events
//
// This example demonstrates subscribing to real-time job events.
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
	"github.com/spooled-cloud/spooled-sdk-go/spooled/realtime"
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

	wsURL := os.Getenv("WS_URL")
	if wsURL == "" {
		wsURL = "wss://api.spooled.cloud/api/v1/ws"
	}

	// Create REST client for creating jobs
	client, err := spooled.NewClient(
		spooled.WithAPIKey(apiKey),
		spooled.WithBaseURL(baseURL),
	)
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}

	ctx := context.Background()
	queueName := fmt.Sprintf("realtime-example-%d", time.Now().Unix())

	// Create WebSocket client
	fmt.Println("Creating WebSocket client...")
	ws := realtime.NewWebSocketClient(realtime.ConnectionOptions{
		WSURL:  wsURL,
		APIKey: apiKey,
		Debug:  true,
		Logger: func(msg string, args ...any) {
			fmt.Printf("[WS] "+msg+"\n", args...)
		},
	})

	// Register event handlers
	ws.OnStateChange(func(state realtime.ConnectionState) {
		fmt.Printf("Connection state changed: %s\n", state)
	})

	ws.OnEvent(func(event *realtime.Event) {
		fmt.Printf("Event received: %s at %v\n", event.Type, event.Timestamp)
	})

	ws.OnJobEvent(realtime.EventJobCreated, func(event *realtime.JobEvent) {
		fmt.Printf("🆕 Job created: %s in queue %s\n", event.JobID, event.QueueName)
	})

	ws.OnJobEvent(realtime.EventJobCompleted, func(event *realtime.JobEvent) {
		fmt.Printf("✓ Job completed: %s\n", event.JobID)
	})

	ws.OnJobEvent(realtime.EventJobFailed, func(event *realtime.JobEvent) {
		fmt.Printf("✗ Job failed: %s - %s\n", event.JobID, event.Error)
	})

	// Connect
	fmt.Println("Connecting to WebSocket...")
	if err := ws.Connect(); err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	fmt.Println("✓ Connected")

	// Subscribe to queue
	fmt.Printf("Subscribing to queue: %s\n", queueName)
	if err := ws.Subscribe(realtime.SubscriptionFilter{QueueName: queueName}); err != nil {
		log.Fatalf("Failed to subscribe: %v", err)
	}
	fmt.Println("✓ Subscribed")

	// Create some jobs to trigger events
	fmt.Println("\nCreating test jobs...")
	for i := 0; i < 3; i++ {
		_, err := client.Jobs().Create(ctx, &resources.CreateJobRequest{
			QueueName: queueName,
			Payload: map[string]any{
				"index": i,
				"time":  time.Now().Format(time.RFC3339),
			},
		})
		if err != nil {
			log.Printf("Failed to create job %d: %v", i, err)
		} else {
			fmt.Printf("Created job %d\n", i)
		}
		time.Sleep(500 * time.Millisecond)
	}

	// Wait for events or signal
	fmt.Println("\nWaiting for events (Ctrl+C to exit)...")
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Auto-stop after 30 seconds
	go func() {
		time.Sleep(30 * time.Second)
		fmt.Println("\nAuto-stopping after 30 seconds...")
		sigCh <- syscall.SIGTERM
	}()

	<-sigCh
	fmt.Println("\nDisconnecting...")
	ws.Disconnect()

	fmt.Println("\n✓ Realtime example complete!")
}
