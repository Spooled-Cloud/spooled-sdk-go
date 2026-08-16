// Example: Webhook Configuration
//
// This example demonstrates creating and managing outgoing webhooks.
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

	// Create a webhook for job events. The secret signs every delivery with an
	// X-Spooled-Signature header the receiver can verify.
	fmt.Println("Creating webhook...")
	secret := "whsec_example_signing_secret"
	webhook, err := client.Webhooks().Create(ctx, &resources.CreateOutgoingWebhookRequest{
		Name: "Job Notifications",
		URL:  "https://your-app.com/webhooks/spooled",
		Events: []resources.WebhookEvent{
			resources.WebhookEventJobCompleted,
			resources.WebhookEventJobFailed,
		},
		Secret: &secret,
	})
	if err != nil {
		log.Fatalf("Failed to create webhook: %v", err)
	}
	fmt.Printf("✓ Created webhook: %s\n", webhook.ID)
	fmt.Printf("  Name: %s\n", webhook.Name)
	fmt.Printf("  URL: %s\n", webhook.URL)
	fmt.Printf("  Events: %v\n", webhook.Events)

	// List webhooks
	fmt.Println("\nListing webhooks...")
	webhooks, err := client.Webhooks().List(ctx)
	if err != nil {
		log.Fatalf("Failed to list webhooks: %v", err)
	}
	fmt.Printf("✓ Found %d webhook(s)\n", len(webhooks))
	for _, wh := range webhooks {
		fmt.Printf("  - %s: %s (%s)\n", wh.ID, wh.Name, wh.URL)
	}

	// Get webhook details
	fmt.Println("\nGetting webhook details...")
	wh, err := client.Webhooks().Get(ctx, webhook.ID)
	if err != nil {
		log.Fatalf("Failed to get webhook: %v", err)
	}
	fmt.Printf("✓ Webhook: %s\n", wh.Name)
	fmt.Printf("  Enabled: %v\n", wh.Enabled)
	fmt.Printf("  Events: %v\n", wh.Events)

	// Health: failure_count counts consecutive failed deliveries (one per
	// delivery, not per retry attempt) and last_status is success, failed or
	// auto_disabled. At 20 consecutive failed deliveries the server disables
	// the webhook itself and it stops receiving events.
	fmt.Printf("  Consecutive failed deliveries: %d\n", wh.FailureCount)
	if wh.LastStatus != nil {
		fmt.Printf("  Last status: %s\n", *wh.LastStatus)
		if *wh.LastStatus == "auto_disabled" {
			fmt.Println("  ⚠️  Auto-disabled after 20 consecutive failed deliveries")

			// Re-enabling is charged against the plan webhook cap, so it can
			// come back as 429 QUOTA_EXCEEDED.
			enabled := true
			if _, err := client.Webhooks().Update(ctx, wh.ID, &resources.UpdateOutgoingWebhookRequest{
				Enabled: &enabled,
			}); err != nil {
				log.Printf("Failed to re-enable webhook: %v", err)
			} else {
				fmt.Println("  ✓ Re-enabled")
			}
		}
	}

	// Update webhook
	fmt.Println("\nUpdating webhook...")
	newEvents := []resources.WebhookEvent{
		resources.WebhookEventJobCompleted,
		resources.WebhookEventJobFailed,
		resources.WebhookEventJobCreated,
	}
	updated, err := client.Webhooks().Update(ctx, webhook.ID, &resources.UpdateOutgoingWebhookRequest{
		Events: &newEvents,
	})
	if err != nil {
		log.Fatalf("Failed to update webhook: %v", err)
	}
	fmt.Printf("✓ Updated webhook events: %v\n", updated.Events)

	// Leaving Secret nil above kept the existing signing secret. Setting
	// ClearSecret instead removes it, and deliveries then go out unsigned with
	// no X-Spooled-Signature header - there is no way to get the old value
	// back, so only do this deliberately.
	fmt.Println("\nRemoving the signing secret...")
	unsigned, err := client.Webhooks().Update(ctx, webhook.ID, &resources.UpdateOutgoingWebhookRequest{
		ClearSecret: true,
	})
	if err != nil {
		log.Fatalf("Failed to clear webhook secret: %v", err)
	}
	fmt.Printf("✓ Deliveries for %s are now unsigned\n", unsigned.Name)

	// Test webhook
	fmt.Println("\nTesting webhook...")
	testResult, err := client.Webhooks().Test(ctx, webhook.ID)
	if err != nil {
		log.Printf("Webhook test failed: %v", err)
	} else {
		fmt.Printf("✓ Test result: success=%v\n", testResult.Success)
		if testResult.StatusCode != nil {
			fmt.Printf("  Status code: %d\n", *testResult.StatusCode)
		}
	}

	// Get webhook deliveries. Only the newest 100 per webhook are readable, and
	// the retention sweep removes rows past the plan's history window (1 day on
	// free, 7 on starter, 30 on pro, 90 on enterprise).
	fmt.Println("\nGetting webhook deliveries...")
	deliveries, err := client.Webhooks().Deliveries(ctx, webhook.ID, nil)
	if err != nil {
		log.Printf("Failed to get deliveries: %v", err)
	} else {
		fmt.Printf("✓ Found %d delivery(ies)\n", len(deliveries))
		for _, d := range deliveries {
			fmt.Printf("  - %s: %s\n", d.ID, d.Status)
		}
	}

	// Delete webhook
	fmt.Println("\nDeleting webhook...")
	err = client.Webhooks().Delete(ctx, webhook.ID)
	if err != nil {
		log.Fatalf("Failed to delete webhook: %v", err)
	}
	fmt.Printf("✓ Webhook deleted\n")

	fmt.Println("\n✓ Webhook example complete!")
}
