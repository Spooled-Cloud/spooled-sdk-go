// Package spooled provides top-level convenience functions for common operations.
//
// These functions provide shortcuts for the most common SDK operations.
package spooled

import (
	"context"
	"time"

	"github.com/spooled-cloud/spooled-sdk-go/spooled/resources"
)

// CreateClient creates a new Spooled client with default configuration.
//
// This is a convenience function equivalent to:
//
//	client, err := spooled.NewClient(spooled.WithAPIKey(apiKey))
//
// Example:
//
//	client, err := spooled.CreateClient("sp_live_your_api_key")
//	if err != nil {
//		log.Fatal(err)
//	}
//	defer client.Close()
func CreateClient(apiKey string) (*Client, error) {
	return NewClient(WithAPIKey(apiKey))
}

// CreateJob creates a job and returns the job ID.
//
// This is a convenience function for simple job creation.
//
// Example:
//
//	jobID, err := spooled.CreateJob(client, "emails", map[string]any{
//		"to": "user@example.com",
//		"subject": "Hello!",
//	})
//	if err != nil {
//		log.Fatal(err)
//	}
func CreateJob(client *Client, queueName string, payload map[string]any) (string, error) {
	resp, err := client.Jobs().Create(context.Background(), &resources.CreateJobRequest{
		QueueName: queueName,
		Payload:   payload,
	})
	if err != nil {
		return "", err
	}
	return resp.ID, nil
}

// CreateJobWithOptions creates a job with full options.
//
// Example:
//
//	jobID, err := spooled.CreateJobWithOptions(client, &resources.CreateJobRequest{
//		QueueName:  "emails",
//		Payload:    map[string]any{"to": "user@example.com"},
//		Priority:   ptr(10),
//		ScheduledAt: &futureTime,
//	})
func CreateJobWithOptions(client *Client, req *resources.CreateJobRequest) (string, error) {
	resp, err := client.Jobs().Create(context.Background(), req)
	if err != nil {
		return "", err
	}
	return resp.ID, nil
}

// GetJob retrieves a job by ID.
//
// Example:
//
//	job, err := spooled.GetJob(client, "job_123")
//	if err != nil {
//		log.Fatal(err)
//	}
//	fmt.Printf("Job status: %s\n", job.Status)
func GetJob(client *Client, jobID string) (*resources.Job, error) {
	return client.Jobs().Get(context.Background(), jobID)
}

// CancelJob cancels a job by ID.
//
// Example:
//
//	err := spooled.CancelJob(client, "job_123")
//	if err != nil {
//		log.Fatal(err)
//	}
func CancelJob(client *Client, jobID string) error {
	return client.Jobs().Cancel(context.Background(), jobID)
}

// ListJobs lists jobs with optional filters.
//
// Example:
//
//	jobs, err := spooled.ListJobs(client, &resources.ListJobsParams{
//		QueueName: &queueName,
//		Status:    &status,
//		Limit:     ptr(50),
//	})
func ListJobs(client *Client, params *resources.ListJobsParams) ([]resources.Job, error) {
	return client.Jobs().List(context.Background(), params)
}

// CreateWorker creates and starts a Spooled worker.
//
// This is a convenience function that creates, configures, and starts a worker.
//
// Example:
//
//	worker, err := spooled.CreateWorker(client, "emails", func(ctx context.Context, job *resources.Job) (any, error) {
//		fmt.Printf("Processing email to: %v\n", job.Payload["to"])
//		return map[string]any{"sent": true}, nil
//	})
//	if err != nil {
//		log.Fatal(err)
//	}
//	defer worker.Stop()
//
//	// Worker runs until stopped
//	select {}
func CreateWorker(client *Client, queueName string, handler func(context.Context, *resources.Job) (any, error)) (*SpooledWorker, error) {
	worker := NewSpooledWorker(client, SpooledWorkerOptions{
		QueueName: queueName,
	})
	worker.Process(handler)

	err := worker.Start()
	if err != nil {
		return nil, err
	}

	return worker, nil
}

// CreateScheduledJob creates a job scheduled for the future.
//
// Example:
//
//	future := time.Now().Add(1 * time.Hour)
//	jobID, err := spooled.CreateScheduledJob(client, "emails", map[string]any{
//		"to": "user@example.com",
//	}, future)
func CreateScheduledJob(client *Client, queueName string, payload map[string]any, scheduledAt time.Time) (string, error) {
	resp, err := client.Jobs().Create(context.Background(), &resources.CreateJobRequest{
		QueueName:   queueName,
		Payload:     payload,
		ScheduledAt: &scheduledAt,
	})
	if err != nil {
		return "", err
	}
	return resp.ID, nil
}

// GetQueueStats retrieves statistics for a queue.
//
// Example:
//
//	stats, err := spooled.GetQueueStats(client, "emails")
//	if err != nil {
//		log.Fatal(err)
//	}
//	fmt.Printf("Pending jobs: %d\n", stats.PendingJobs)
func GetQueueStats(client *Client, queueName string) (*resources.QueueStats, error) {
	return client.Queues().GetStats(context.Background(), queueName)
}

// RegisterWorker registers a new worker.
//
// Example:
//
//	resp, err := spooled.RegisterWorker(client, &resources.RegisterWorkerRequest{
//		QueueName: "emails",
//		Hostname:  "worker-01",
//	})
//	if err != nil {
//		log.Fatal(err)
//	}
//	fmt.Printf("Registered worker: %s\n", resp.ID)
func RegisterWorker(client *Client, req *resources.RegisterWorkerRequest) (*resources.RegisterWorkerResponse, error) {
	return client.Workers().Register(context.Background(), req)
}

// ListWorkers lists all workers.
//
// Example:
//
//	workers, err := spooled.ListWorkers(client)
//	if err != nil {
//		log.Fatal(err)
//	}
//	for _, w := range workers {
//		fmt.Printf("Worker: %s (%s)\n", w.ID, w.Status)
//	}
func ListWorkers(client *Client) ([]resources.Worker, error) {
	return client.Workers().List(context.Background())
}

// CreateWebhook creates an outgoing webhook.
//
// Example:
//
//	webhook, err := spooled.CreateWebhook(client, &resources.CreateOutgoingWebhookRequest{
//		Name:   "email-events",
//		URL:    "https://api.example.com/webhooks",
//		Events: []resources.WebhookEvent{resources.WebhookEventJobCompleted},
//	})
//	if err != nil {
//		log.Fatal(err)
//	}
func CreateWebhook(client *Client, req *resources.CreateOutgoingWebhookRequest) (*resources.OutgoingWebhook, error) {
	return client.Webhooks().Create(context.Background(), req)
}

// GetDashboard retrieves dashboard data.
//
// Example:
//
//	dashboard, err := spooled.GetDashboard(client)
//	if err != nil {
//		log.Fatal(err)
//	}
//	fmt.Printf("Total jobs: %d\n", dashboard.Jobs.Total)
func GetDashboard(client *Client) (*resources.DashboardData, error) {
	return client.Dashboard().Get(context.Background())
}

// GetHealth checks the health of the Spooled service.
//
// Example:
//
//	health, err := spooled.GetHealth(client)
//	if err != nil {
//		log.Fatal(err)
//	}
//	fmt.Printf("Status: %s\n", health.Status)
func GetHealth(client *Client) (*resources.HealthResponse, error) {
	return client.Health().Get(context.Background())
}
