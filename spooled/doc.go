// Package spooled provides the official Go SDK for Spooled Cloud.
//
// Spooled Cloud is a modern, scalable job queue and task scheduler service.
// This SDK provides a complete interface for interacting with all Spooled API
// endpoints, including job management, queue operations, worker registration,
// real-time events, scheduling, and workflows.
//
// # Basic Usage
//
// Create a client with your API key:
//
//	client, err := spooled.NewClient(
//		spooled.WithAPIKey("sp_live_your_api_key"),
//	)
//	if err != nil {
//		log.Fatal(err)
//	}
//	defer client.Close()
//
// # Creating Jobs
//
//	ctx := context.Background()
//	resp, err := client.Jobs().Create(ctx, &types.CreateJobRequest{
//		QueueName: "emails",
//		Payload: map[string]any{
//			"to":      "user@example.com",
//			"subject": "Hello!",
//		},
//	})
//	if err != nil {
//		log.Fatal(err)
//	}
//	fmt.Printf("Created job: %s\n", resp.ID)
//
// # Configuration Options
//
// The client supports various configuration options:
//
//	client, err := spooled.NewClient(
//		spooled.WithAPIKey("sp_live_..."),
//		spooled.WithBaseURL("https://api.spooled.cloud"),
//		spooled.WithTimeout(30 * time.Second),
//		spooled.WithRetry(spooled.RetryConfig{
//			MaxRetries: 3,
//			BaseDelay:  time.Second,
//		}),
//		spooled.WithDebug(true),
//	)
//
// # Error Handling
//
// All SDK errors implement the error interface and can be inspected:
//
//	job, err := client.Jobs().Get(ctx, "job-id")
//	if err != nil {
//		if spooled.IsNotFoundError(err) {
//			fmt.Println("Job not found")
//		} else if spooled.IsRateLimitError(err) {
//			var rateLimitErr *spooled.RateLimitError
//			if errors.As(err, &rateLimitErr) {
//				fmt.Printf("Rate limited, retry after %d seconds\n", rateLimitErr.GetRetryAfter())
//			}
//		} else {
//			log.Fatal(err)
//		}
//	}
//
// # Resources
//
// The client provides access to various resources:
//
//   - Jobs: Create, list, cancel, retry jobs and manage the dead letter queue
//   - Queues: List, configure, pause/resume queues
//   - Workers: Register, heartbeat, and deregister workers
//   - Schedules: Create and manage scheduled jobs
//   - Workflows: Create and manage job workflows with dependencies
//   - Webhooks: Configure outgoing webhooks for events
//   - Organizations: Manage organizations and usage
//   - API Keys: Manage API keys
//   - Billing: Access billing status and portal
//   - Auth: Authentication and token management
package spooled
