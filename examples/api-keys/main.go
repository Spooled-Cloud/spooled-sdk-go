// Example: API Key Management
//
// This example demonstrates creating and managing API keys.
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

	// List existing API keys
	fmt.Println("Listing API keys...")
	keys, err := client.APIKeys().List(ctx)
	if err != nil {
		log.Fatalf("Failed to list API keys: %v", err)
	}
	fmt.Printf("✓ Found %d API key(s)\n", len(keys))
	for _, key := range keys {
		prefix := ""
		if key.KeyPrefix != nil {
			prefix = *key.KeyPrefix
		}
		fmt.Printf("  - %s: %s (prefix: %s)\n", key.ID, key.Name, prefix)
	}

	// Create a new API key
	fmt.Println("\nCreating new API key...")
	newKey, err := client.APIKeys().Create(ctx, &resources.CreateAPIKeyRequest{
		Name: "Example Key",
	})
	if err != nil {
		log.Fatalf("Failed to create API key: %v", err)
	}
	fmt.Printf("✓ Created API key: %s\n", newKey.ID)
	fmt.Printf("  Name: %s\n", newKey.Name)
	fmt.Printf("  Key: %s\n", newKey.Key) // Only shown once!
	fmt.Println("  ⚠️  Save the key now - it won't be shown again!")

	// Get API key details
	fmt.Println("\nGetting API key details...")
	key, err := client.APIKeys().Get(ctx, newKey.ID)
	if err != nil {
		log.Fatalf("Failed to get API key: %v", err)
	}
	fmt.Printf("✓ API key: %s\n", key.Name)
	fmt.Printf("  ID: %s\n", key.ID)
	fmt.Printf("  Active: %v\n", key.IsActive)
	fmt.Printf("  Created: %v\n", key.CreatedAt)
	if key.LastUsed != nil {
		fmt.Printf("  Last used: %v\n", *key.LastUsed)
	}

	// Update API key
	fmt.Println("\nUpdating API key...")
	newName := "Updated Example Key"
	updated, err := client.APIKeys().Update(ctx, newKey.ID, &resources.UpdateAPIKeyRequest{
		Name: &newName,
	})
	if err != nil {
		log.Fatalf("Failed to update API key: %v", err)
	}
	fmt.Printf("✓ Updated API key name: %s\n", updated.Name)

	// Test the new API key works
	fmt.Println("\nTesting new API key...")
	testClient, err := spooled.NewClient(
		spooled.WithAPIKey(newKey.Key),
		spooled.WithBaseURL(baseURL),
	)
	if err != nil {
		log.Printf("Failed to create test client: %v", err)
	} else {
		health, err := testClient.Health().Get(ctx)
		if err != nil {
			log.Printf("New API key test failed: %v", err)
		} else {
			fmt.Printf("✓ New API key works! Health: %s\n", health.Status)
		}
	}

	// Delete the API key
	fmt.Println("\nDeleting API key...")
	err = client.APIKeys().Delete(ctx, newKey.ID)
	if err != nil {
		log.Fatalf("Failed to delete API key: %v", err)
	}
	fmt.Printf("✓ API key deleted\n")

	// Verify the deleted key no longer works
	fmt.Println("\nVerifying deleted key...")
	_, err = testClient.Health().Get(ctx)
	if err != nil {
		fmt.Printf("✓ Deleted key correctly rejected: %v\n", err)
	} else {
		fmt.Println("⚠️  Deleted key still works (may take time to propagate)")
	}

	fmt.Println("\n✓ API key example complete!")
}
