// Example: Workflow DAGs
//
// This example demonstrates creating workflows with job dependencies.
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
	queueName := fmt.Sprintf("workflow-example-%d", time.Now().Unix())

	// Create a workflow: ETL Pipeline
	// extract -> transform -> load
	fmt.Println("Creating ETL Pipeline workflow...")
	workflow, err := client.Workflows().Create(ctx, &resources.CreateWorkflowRequest{
		Name: "ETL Pipeline Example",
		Jobs: []resources.WorkflowJobDefinition{
			{
				Key:       "extract",
				QueueName: queueName,
				Payload: map[string]any{
					"step":   "extract",
					"source": "database",
				},
			},
			{
				Key:       "transform",
				QueueName: queueName,
				Payload: map[string]any{
					"step":       "transform",
					"operations": []string{"clean", "normalize", "enrich"},
				},
				DependsOn: []string{"extract"},
			},
			{
				Key:       "load",
				QueueName: queueName,
				Payload: map[string]any{
					"step":        "load",
					"destination": "data-warehouse",
				},
				DependsOn: []string{"transform"},
			},
		},
	})
	if err != nil {
		log.Fatalf("Failed to create workflow: %v", err)
	}

	fmt.Printf("✓ Workflow created: %s\n", workflow.WorkflowID)
	fmt.Printf("  Jobs created: %d\n", len(workflow.JobIDs))
	for _, job := range workflow.JobIDs {
		fmt.Printf("    - %s: %s\n", job.Key, job.JobID)
	}
	fmt.Println()

	// Get workflow details
	fmt.Println("Getting workflow details...")
	wf, err := client.Workflows().Get(ctx, workflow.WorkflowID)
	if err != nil {
		log.Fatalf("Failed to get workflow: %v", err)
	}

	fmt.Printf("Workflow: %s\n", wf.Name)
	fmt.Printf("Status: %s\n", wf.Status)
	fmt.Printf("Total Jobs: %d\n", wf.TotalJobs)
	fmt.Printf("Completed: %d, Failed: %d\n", wf.CompletedJobs, wf.FailedJobs)

	// Get jobs in workflow
	jobs, err := client.Workflows().Jobs().ListJobs(ctx, workflow.WorkflowID)
	if err == nil {
		fmt.Println("Jobs:")
		for _, job := range jobs {
			fmt.Printf("  - %s (%s): %s\n", job.Key, job.ID, job.Status)
		}
	}

	// Create a more complex workflow: Fan-out/Fan-in
	fmt.Println("\nCreating Fan-out/Fan-in workflow...")
	fanWorkflow, err := client.Workflows().Create(ctx, &resources.CreateWorkflowRequest{
		Name: "Fan-out Fan-in Example",
		Jobs: []resources.WorkflowJobDefinition{
			{
				Key:       "split",
				QueueName: queueName,
				Payload:   map[string]any{"action": "split"},
			},
			{
				Key:       "process-1",
				QueueName: queueName,
				Payload:   map[string]any{"action": "process", "partition": 1},
				DependsOn: []string{"split"},
			},
			{
				Key:       "process-2",
				QueueName: queueName,
				Payload:   map[string]any{"action": "process", "partition": 2},
				DependsOn: []string{"split"},
			},
			{
				Key:       "process-3",
				QueueName: queueName,
				Payload:   map[string]any{"action": "process", "partition": 3},
				DependsOn: []string{"split"},
			},
			{
				Key:       "merge",
				QueueName: queueName,
				Payload:   map[string]any{"action": "merge"},
				DependsOn: []string{"process-1", "process-2", "process-3"},
			},
		},
	})
	if err != nil {
		log.Fatalf("Failed to create fan workflow: %v", err)
	}

	fmt.Printf("✓ Fan-out/Fan-in workflow created: %s\n", fanWorkflow.WorkflowID)
	fmt.Printf("  Jobs created: %d\n", len(fanWorkflow.JobIDs))

	// List workflows
	fmt.Println("\nListing workflows...")
	limit := 10
	workflows, err := client.Workflows().List(ctx, &resources.ListWorkflowsParams{
		Limit: &limit,
	})
	if err != nil {
		log.Fatalf("Failed to list workflows: %v", err)
	}
	fmt.Printf("✓ Found %d workflow(s)\n", len(workflows))

	fmt.Println("\n✓ Workflow example complete!")
}
