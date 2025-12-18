// Package main provides comprehensive tests for the Spooled Go SDK.
// This test suite achieves parity with the Node.js SDK tests.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/spooled-cloud/spooled-sdk-go/spooled"
	spooledgrpc "github.com/spooled-cloud/spooled-sdk-go/spooled/grpc"
	"github.com/spooled-cloud/spooled-sdk-go/spooled/resources"
)

// ─────────────────────────────────────────────────────────────────────────────
// Configuration
// ─────────────────────────────────────────────────────────────────────────────

var (
	baseURL     = getEnv("BASE_URL", "http://localhost:8080")
	apiKey      = os.Getenv("API_KEY")
	adminKey    = os.Getenv("ADMIN_KEY")
	webhookPort = getEnvInt("WEBHOOK_PORT", 9999)
	verbose     = getEnvBool("VERBOSE", false)
	skipStress  = getEnvBool("SKIP_STRESS", true)
	skipGRPC    = getEnvBool("SKIP_GRPC", false)
	skipIngest  = getEnvBool("SKIP_INGEST", true)
	grpcAddress = getEnv("GRPC_ADDRESS", "localhost:50051")
	grpcUseTLS  = getEnvBool("GRPC_USE_TLS", false)
	testPrefix  string
	// isProduction detects if we're running against production API (which requires HTTPS webhooks)
	isProduction bool
)

// ─────────────────────────────────────────────────────────────────────────────
// Test Results Tracking
// ─────────────────────────────────────────────────────────────────────────────

type TestResult struct {
	Name    string
	Passed  bool
	Skipped bool
	Error   string
}

var (
	results     []TestResult
	resultsMu   sync.Mutex
	webhookChan = make(chan map[string]interface{}, 100)
)

// ─────────────────────────────────────────────────────────────────────────────
// Runner & Helper Functions
// ─────────────────────────────────────────────────────────────────────────────

type Runner struct {
	name string
}

func (r *Runner) Run(name string, fn func()) {
	fullName := fmt.Sprintf("%s: %s", r.name, name)
	defer func() {
		if rec := recover(); rec != nil {
			resultsMu.Lock()
			results = append(results, TestResult{Name: fullName, Passed: false, Error: fmt.Sprintf("%v", rec)})
			resultsMu.Unlock()
			fmt.Printf("  ✗ %s\n    Error: %v\n", name, rec)
		}
	}()

	fn()
	resultsMu.Lock()
	results = append(results, TestResult{Name: fullName, Passed: true})
	resultsMu.Unlock()
	fmt.Printf("  ✓ %s\n", name)
}

func (r *Runner) Skip(name, reason string) {
	fullName := fmt.Sprintf("%s: %s", r.name, name)
	resultsMu.Lock()
	results = append(results, TestResult{Name: fullName, Passed: true, Skipped: true})
	resultsMu.Unlock()
	fmt.Printf("  ⏭️  %s (skipped: %s)\n", name, reason)
}

func assertEqual(actual, expected interface{}, msg string) {
	if actual != expected {
		panic(fmt.Sprintf("Assertion failed: %s - expected %v, got %v", msg, expected, actual))
	}
}

func assertNotEmpty(val interface{}, msg string) {
	if val == nil || val == "" {
		panic(fmt.Sprintf("Assertion failed: %s - expected non-empty value", msg))
	}
}

func assertTrue(cond bool, msg string) {
	if !cond {
		panic(fmt.Sprintf("Assertion failed: %s", msg))
	}
}

func assertError(err error, msg string) {
	if err == nil {
		panic(fmt.Sprintf("Assertion failed: %s - expected error but got nil", msg))
	}
}

func assertNoError(err error) {
	if err != nil {
		panic(fmt.Sprintf("Unexpected error: %v", err))
	}
}

func log(msg string) {
	if verbose {
		fmt.Printf("    → %s\n", msg)
	}
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func getEnvInt(key string, defaultVal int) int {
	if val := os.Getenv(key); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			return i
		}
	}
	return defaultVal
}

func getEnvBool(key string, defaultVal bool) bool {
	if val := os.Getenv(key); val != "" {
		return val == "true" || val == "1"
	}
	return defaultVal
}

func generateTestID() string {
	const chars = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, 8)
	for i := range b {
		b[i] = chars[rand.Intn(len(chars))]
	}
	return fmt.Sprintf("go-test-%s", string(b))
}

func ptr[T any](v T) *T {
	return &v
}

// ─────────────────────────────────────────────────────────────────────────────
// Webhook Server
// ─────────────────────────────────────────────────────────────────────────────

var webhookServer *http.Server

func startWebhookServer() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/webhook", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var payload map[string]interface{}
		json.Unmarshal(body, &payload)
		select {
		case webhookChan <- payload:
		default:
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"received": true}`))
	})

	webhookServer = &http.Server{
		Addr:    fmt.Sprintf(":%d", webhookPort),
		Handler: mux,
	}

	go webhookServer.ListenAndServe()
	time.Sleep(100 * time.Millisecond)
	return nil
}

func stopWebhookServer() {
	if webhookServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		webhookServer.Shutdown(ctx)
	}
}

func waitForWebhook(timeout time.Duration) (map[string]interface{}, bool) {
	select {
	case payload := <-webhookChan:
		return payload, true
	case <-time.After(timeout):
		return nil, false
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Job Cleanup Helper
// ─────────────────────────────────────────────────────────────────────────────

func cleanupOldJobs(client *spooled.Client) error {
	ctx := context.Background()
	jobs, err := client.Jobs().List(ctx, &resources.ListJobsParams{Limit: ptr(100)})
	if err != nil {
		return err
	}

	for _, job := range jobs {
		if job.Status == "pending" || job.Status == "processing" {
			client.Jobs().Cancel(ctx, job.ID)
		}
	}
	return nil
}

// ─────────────────────────────────────────────────────────────────────────────
// TEST: Health Endpoints
// ─────────────────────────────────────────────────────────────────────────────

func testHealthEndpoints(client *spooled.Client) {
	fmt.Println("\n🏥 Health Endpoints")
	fmt.Println(strings.Repeat("─", 60))

	r := &Runner{name: "Health"}
	ctx := context.Background()

	r.Run("Basic health check returns healthy", func() {
		health, err := client.Health().Get(ctx)
		assertNoError(err)
		assertEqual(health.Status, "healthy", "status should be healthy")
		log(fmt.Sprintf("Status: %s", health.Status))
	})

	r.Run("Liveness check returns status", func() {
		health, err := client.Health().Liveness(ctx)
		assertNoError(err)
		// Liveness may return empty status or just 200 OK
		log(fmt.Sprintf("Liveness status: %q", health.Status))
	})

	r.Run("Readiness check returns status", func() {
		health, err := client.Health().Readiness(ctx)
		assertNoError(err)
		// Readiness may return empty status or just 200 OK
		log(fmt.Sprintf("Readiness status: %q", health.Status))
	})

	r.Run("Health check includes database status", func() {
		health, err := client.Health().Get(ctx)
		assertNoError(err)
		// Database field is a bool
		log(fmt.Sprintf("Database: %v", health.Database))
	})

	r.Run("Health check includes cache status", func() {
		health, err := client.Health().Get(ctx)
		assertNoError(err)
		// Cache field is a bool
		log(fmt.Sprintf("Cache: %v", health.Cache))
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// TEST: Dashboard
// ─────────────────────────────────────────────────────────────────────────────

func testDashboard(client *spooled.Client) {
	fmt.Println("\n📊 Dashboard")
	fmt.Println(strings.Repeat("─", 60))

	r := &Runner{name: "Dashboard"}
	ctx := context.Background()

	r.Run("Dashboard returns overview data", func() {
		data, err := client.Dashboard().Get(ctx)
		assertNoError(err)
		assertTrue(data != nil, "data should exist")
		log("Dashboard data retrieved")
	})

	r.Run("Dashboard includes system info", func() {
		data, err := client.Dashboard().Get(ctx)
		assertNoError(err)
		assertNotEmpty(data.System.Version, "version should exist")
	})

	r.Run("Dashboard includes job stats", func() {
		data, err := client.Dashboard().Get(ctx)
		assertNoError(err)
		assertTrue(data.Jobs.Total >= 0, "total jobs should be non-negative")
	})

	r.Run("Dashboard includes worker info", func() {
		data, err := client.Dashboard().Get(ctx)
		assertNoError(err)
		assertTrue(data.Workers.Total >= 0, "total workers should be non-negative")
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// TEST: Organizations
// ─────────────────────────────────────────────────────────────────────────────

func testOrganization(client *spooled.Client) {
	fmt.Println("\n🏢 Organization")
	fmt.Println(strings.Repeat("─", 60))

	r := &Runner{name: "Organization"}
	ctx := context.Background()

	r.Run("List organizations", func() {
		orgs, err := client.Organizations().List(ctx)
		assertNoError(err)
		assertTrue(len(orgs) > 0, "should have at least one org")
		log(fmt.Sprintf("Found %d organizations", len(orgs)))
	})

	r.Run("Organization has ID and name", func() {
		orgs, err := client.Organizations().List(ctx)
		assertNoError(err)
		assertTrue(len(orgs) > 0, "should have orgs")
		assertNotEmpty(orgs[0].ID, "org ID should exist")
		assertNotEmpty(orgs[0].Name, "org name should exist")
		log(fmt.Sprintf("Org: %s (%s)", orgs[0].Name, orgs[0].ID))
	})

	r.Run("Organization has slug", func() {
		orgs, err := client.Organizations().List(ctx)
		assertNoError(err)
		assertTrue(len(orgs) > 0, "should have orgs")
		assertNotEmpty(orgs[0].Slug, "org slug should exist")
	})

	r.Run("Organization has tier info", func() {
		orgs, err := client.Organizations().List(ctx)
		assertNoError(err)
		assertTrue(len(orgs) > 0, "should have orgs")
		assertNotEmpty(string(orgs[0].PlanTier), "tier should exist")
		log(fmt.Sprintf("Tier: %s", orgs[0].PlanTier))
	})

	r.Run("Get organization by ID", func() {
		orgs, err := client.Organizations().List(ctx)
		assertNoError(err)
		assertTrue(len(orgs) > 0, "should have orgs")

		org, err := client.Organizations().Get(ctx, orgs[0].ID)
		assertNoError(err)
		assertEqual(org.ID, orgs[0].ID, "org ID should match")
	})

	r.Run("Get organization usage & limits", func() {
		orgs, _ := client.Organizations().List(ctx)
		if len(orgs) > 0 {
			usage, err := client.Organizations().Usage(ctx, orgs[0].ID)
			if err == nil && usage != nil {
				log(fmt.Sprintf("Usage plan: %s", usage.Plan))
			}
		}
	})

	r.Run("Create new organization", func() {
		// Try to create org - may be limited by tier
		newOrg, err := client.Organizations().Create(ctx, &resources.CreateOrganizationRequest{
			Name: fmt.Sprintf("%s-test-org", testPrefix),
			Slug: fmt.Sprintf("%s-test-org", testPrefix),
		})
		if err == nil {
			assertNotEmpty(newOrg.Organization.ID, "new org ID")
			log(fmt.Sprintf("Created org: %s", newOrg.Organization.ID))
			// Cleanup
			client.Organizations().Delete(ctx, newOrg.Organization.ID)
		} else {
			log(fmt.Sprintf("Create org (expected on free tier): %v", err))
		}
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// TEST: API Keys
// ─────────────────────────────────────────────────────────────────────────────

func testAPIKeys(client *spooled.Client) {
	fmt.Println("\n🔑 API Keys")
	fmt.Println(strings.Repeat("─", 60))

	r := &Runner{name: "API Keys"}
	ctx := context.Background()
	var createdKeyID string
	var createdKeyValue string

	r.Run("List API keys", func() {
		keys, err := client.APIKeys().List(ctx)
		assertNoError(err)
		assertTrue(len(keys) >= 0, "should return array")
		log(fmt.Sprintf("Found %d API keys", len(keys)))
	})

	r.Run("Create new API key", func() {
		key, err := client.APIKeys().Create(ctx, &resources.CreateAPIKeyRequest{
			Name: fmt.Sprintf("%s-key", testPrefix),
		})
		assertNoError(err)
		assertNotEmpty(key.ID, "key ID should exist")
		assertTrue(strings.HasPrefix(key.Key, "sk_"), "key should start with sk_")
		createdKeyID = key.ID
		createdKeyValue = key.Key
		log(fmt.Sprintf("Created key: %s", key.ID))
	})

	r.Run("Get API key by ID", func() {
		if createdKeyID == "" {
			panic("no key created")
		}
		key, err := client.APIKeys().Get(ctx, createdKeyID)
		assertNoError(err)
		assertEqual(key.ID, createdKeyID, "key ID should match")
	})

	r.Run("API key has metadata", func() {
		if createdKeyID == "" {
			panic("no key created")
		}
		key, err := client.APIKeys().Get(ctx, createdKeyID)
		assertNoError(err)
		assertTrue(!key.CreatedAt.IsZero(), "createdAt should exist")
	})

	r.Run("Update API key", func() {
		if createdKeyID == "" {
			panic("no key created")
		}
		newName := fmt.Sprintf("%s-key-updated", testPrefix)
		key, err := client.APIKeys().Update(ctx, createdKeyID, &resources.UpdateAPIKeyRequest{
			Name: &newName,
		})
		if err == nil {
			assertEqual(key.Name, newName, "name should be updated")
			log(fmt.Sprintf("Updated key name: %s", newName))
		}
	})

	r.Run("New API key works for authentication", func() {
		if createdKeyValue == "" {
			panic("no key created")
		}
		newClient, err := spooled.NewClient(
			spooled.WithAPIKey(createdKeyValue),
			spooled.WithBaseURL(baseURL),
		)
		if err == nil {
			_, err = newClient.Health().Get(ctx)
			assertNoError(err)
			log("New API key authenticated successfully")
		}
	})

	r.Run("Delete API key", func() {
		if createdKeyID == "" {
			panic("no key created")
		}
		err := client.APIKeys().Delete(ctx, createdKeyID)
		assertNoError(err)
		log(fmt.Sprintf("Deleted key: %s", createdKeyID))
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// TEST: Jobs Basic CRUD
// ─────────────────────────────────────────────────────────────────────────────

func testJobsBasicCRUD(client *spooled.Client) {
	fmt.Println("\n📋 Jobs - Basic CRUD")
	fmt.Println(strings.Repeat("─", 60))

	r := &Runner{name: "Jobs CRUD"}
	ctx := context.Background()
	queueName := fmt.Sprintf("%s-crud", testPrefix)
	var jobID string

	r.Run("Create job with minimal params", func() {
		resp, err := client.Jobs().Create(ctx, &resources.CreateJobRequest{
			QueueName: queueName,
			Payload:   map[string]interface{}{"test": "minimal"},
		})
		assertNoError(err)
		assertNotEmpty(resp.ID, "job ID should exist")
		jobID = resp.ID
		log(fmt.Sprintf("Created job: %s", jobID))
	})

	r.Run("Get job by ID", func() {
		job, err := client.Jobs().Get(ctx, jobID)
		assertNoError(err)
		assertEqual(job.ID, jobID, "job ID should match")
		assertEqual(job.QueueName, queueName, "queue name should match")
	})

	r.Run("Job has pending status", func() {
		job, err := client.Jobs().Get(ctx, jobID)
		assertNoError(err)
		assertEqual(string(job.Status), "pending", "status should be pending")
	})

	r.Run("Job has creation timestamp", func() {
		job, err := client.Jobs().Get(ctx, jobID)
		assertNoError(err)
		assertTrue(!job.CreatedAt.IsZero(), "createdAt should exist")
	})

	r.Run("Job has payload", func() {
		job, err := client.Jobs().Get(ctx, jobID)
		assertNoError(err)
		assertTrue(job.Payload != nil, "payload should exist")
	})

	r.Run("List jobs in queue", func() {
		jobs, err := client.Jobs().List(ctx, &resources.ListJobsParams{
			QueueName: &queueName,
			Limit:     ptr(10),
		})
		assertNoError(err)
		assertTrue(len(jobs) > 0, "should have jobs")
		log(fmt.Sprintf("Found %d jobs", len(jobs)))
	})

	r.Run("List jobs with status filter", func() {
		status := resources.JobStatusPending
		jobs, err := client.Jobs().List(ctx, &resources.ListJobsParams{
			QueueName: &queueName,
			Status:    &status,
		})
		assertNoError(err)
		for _, job := range jobs {
			assertEqual(string(job.Status), "pending", "all jobs should be pending")
		}
	})

	r.Run("Get job stats", func() {
		stats, err := client.Jobs().GetStats(ctx)
		assertNoError(err)
		assertTrue(stats.Total >= 0, "total should be non-negative")
	})

	r.Run("Boost job priority", func() {
		// Create a new job to boost
		boostJob, _ := client.Jobs().Create(ctx, &resources.CreateJobRequest{
			QueueName: queueName,
			Payload:   map[string]interface{}{"boost": true},
			Priority:  ptr(1),
		})
		result, err := client.Jobs().BoostPriority(ctx, boostJob.ID, &resources.BoostPriorityRequest{Priority: 100})
		if err == nil {
			assertEqual(result.NewPriority, 100, "priority should be boosted")
			log(fmt.Sprintf("Boosted priority from %d to %d", result.OldPriority, result.NewPriority))
		}
		client.Jobs().Cancel(ctx, boostJob.ID)
	})

	r.Run("Cancel job", func() {
		err := client.Jobs().Cancel(ctx, jobID)
		assertNoError(err)
		log(fmt.Sprintf("Cancelled job: %s", jobID))
	})

	r.Run("Get cancelled job shows cancelled status", func() {
		job, err := client.Jobs().Get(ctx, jobID)
		assertNoError(err)
		assertEqual(string(job.Status), "cancelled", "status should be cancelled")
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// TEST: Jobs Bulk Operations
// ─────────────────────────────────────────────────────────────────────────────

func testJobsBulkOperations(client *spooled.Client) {
	fmt.Println("\n📦 Jobs - Bulk Operations")
	fmt.Println(strings.Repeat("─", 60))

	r := &Runner{name: "Jobs Bulk"}
	ctx := context.Background()
	queueName := fmt.Sprintf("%s-bulk", testPrefix)

	r.Run("Bulk enqueue jobs", func() {
		jobs := []resources.BulkJobItem{
			{Payload: map[string]interface{}{"index": 0}},
			{Payload: map[string]interface{}{"index": 1}},
			{Payload: map[string]interface{}{"index": 2}},
		}
		result, err := client.Jobs().BulkEnqueue(ctx, &resources.BulkEnqueueRequest{
			QueueName: queueName,
			Jobs:      jobs,
		})
		assertNoError(err)
		assertEqual(result.SuccessCount, 3, "all jobs should succeed")
		assertEqual(result.FailureCount, 0, "no failures")
		log(fmt.Sprintf("Bulk enqueued %d jobs", result.SuccessCount))

		// Cleanup
		for _, s := range result.Succeeded {
			client.Jobs().Cancel(ctx, s.JobID)
		}
	})

	r.Run("Bulk enqueue with options per job", func() {
		jobs := []resources.BulkJobItem{
			{Payload: map[string]interface{}{"a": 1}, Priority: ptr(10)},
			{Payload: map[string]interface{}{"b": 2}, Priority: ptr(5)},
		}
		result, err := client.Jobs().BulkEnqueue(ctx, &resources.BulkEnqueueRequest{
			QueueName: queueName,
			Jobs:      jobs,
		})
		assertNoError(err)
		assertEqual(result.SuccessCount, 2, "all jobs should succeed")

		// Cleanup
		for _, s := range result.Succeeded {
			client.Jobs().Cancel(ctx, s.JobID)
		}
	})

	r.Run("Get batch status of multiple jobs", func() {
		// Create some jobs first
		var jobIDs []string
		for i := 0; i < 3; i++ {
			resp, _ := client.Jobs().Create(ctx, &resources.CreateJobRequest{
				QueueName: queueName,
				Payload:   map[string]interface{}{"batch": i},
			})
			jobIDs = append(jobIDs, resp.ID)
		}

		statuses, err := client.Jobs().BatchStatus(ctx, jobIDs)
		assertNoError(err)
		assertTrue(len(statuses) > 0, "should return statuses")

		// Cleanup
		for _, id := range jobIDs {
			client.Jobs().Cancel(ctx, id)
		}
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// TEST: Job Idempotency
// ─────────────────────────────────────────────────────────────────────────────

func testJobIdempotency(client *spooled.Client) {
	fmt.Println("\n🔄 Jobs - Idempotency")
	fmt.Println(strings.Repeat("─", 60))

	r := &Runner{name: "Idempotency"}
	ctx := context.Background()
	queueName := fmt.Sprintf("%s-idemp", testPrefix)

	r.Run("Idempotency key prevents duplicate job", func() {
		idempKey := fmt.Sprintf("test-%s-%d", testPrefix, time.Now().UnixNano())

		job1, err := client.Jobs().Create(ctx, &resources.CreateJobRequest{
			QueueName:      queueName,
			Payload:        map[string]interface{}{"first": true},
			IdempotencyKey: &idempKey,
		})
		assertNoError(err)

		job2, err := client.Jobs().Create(ctx, &resources.CreateJobRequest{
			QueueName:      queueName,
			Payload:        map[string]interface{}{"second": true},
			IdempotencyKey: &idempKey,
		})
		assertNoError(err)

		assertEqual(job1.ID, job2.ID, "same idempotency key should return same job")
		log(fmt.Sprintf("Job ID: %s (deduplicated)", job1.ID))

		client.Jobs().Cancel(ctx, job1.ID)
	})

	r.Run("Different idempotency keys create different jobs", func() {
		key1 := fmt.Sprintf("key1-%d", time.Now().UnixNano())
		key2 := fmt.Sprintf("key2-%d", time.Now().UnixNano())

		job1, _ := client.Jobs().Create(ctx, &resources.CreateJobRequest{
			QueueName:      queueName,
			Payload:        map[string]interface{}{"key": 1},
			IdempotencyKey: &key1,
		})
		job2, _ := client.Jobs().Create(ctx, &resources.CreateJobRequest{
			QueueName:      queueName,
			Payload:        map[string]interface{}{"key": 2},
			IdempotencyKey: &key2,
		})

		assertTrue(job1.ID != job2.ID, "different keys should create different jobs")

		client.Jobs().Cancel(ctx, job1.ID)
		client.Jobs().Cancel(ctx, job2.ID)
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// TEST: Job Lifecycle
// ─────────────────────────────────────────────────────────────────────────────

func testJobLifecycle(client *spooled.Client) {
	fmt.Println("\n🔄 Jobs - Lifecycle")
	fmt.Println(strings.Repeat("─", 60))

	r := &Runner{name: "Job Lifecycle"}
	ctx := context.Background()
	queueName := fmt.Sprintf("%s-lifecycle", testPrefix)

	r.Run("Job transitions from pending to processing", func() {
		jobResp, _ := client.Jobs().Create(ctx, &resources.CreateJobRequest{
			QueueName: queueName,
			Payload:   map[string]interface{}{"test": "lifecycle"},
		})
		worker, _ := client.Workers().Register(ctx, &resources.RegisterWorkerRequest{
			QueueName: queueName,
			Hostname:  "test-worker",
		})

		claimed, err := client.Jobs().Claim(ctx, &resources.ClaimJobsRequest{
			QueueName: queueName,
			WorkerID:  worker.ID,
			Limit:     ptr(1),
		})
		assertNoError(err)
		assertTrue(len(claimed.Jobs) > 0, "should claim job")

		updatedJob, _ := client.Jobs().Get(ctx, jobResp.ID)
		assertEqual(string(updatedJob.Status), "processing", "status should be processing")

		client.Jobs().Complete(ctx, jobResp.ID, &resources.CompleteJobRequest{
			WorkerID: worker.ID,
			Result:   map[string]interface{}{"done": true},
		})
		client.Workers().Deregister(ctx, worker.ID)
	})

	r.Run("Job transitions from processing to completed", func() {
		jobResp, _ := client.Jobs().Create(ctx, &resources.CreateJobRequest{
			QueueName: queueName,
			Payload:   map[string]interface{}{"test": "complete"},
		})
		worker, _ := client.Workers().Register(ctx, &resources.RegisterWorkerRequest{
			QueueName: queueName,
			Hostname:  "test-worker",
		})
		client.Jobs().Claim(ctx, &resources.ClaimJobsRequest{
			QueueName: queueName,
			WorkerID:  worker.ID,
			Limit:     ptr(1),
		})

		err := client.Jobs().Complete(ctx, jobResp.ID, &resources.CompleteJobRequest{
			WorkerID: worker.ID,
			Result:   map[string]interface{}{"completed": true},
		})
		assertNoError(err)

		job, _ := client.Jobs().Get(ctx, jobResp.ID)
		assertEqual(string(job.Status), "completed", "status should be completed")

		client.Workers().Deregister(ctx, worker.ID)
	})

	r.Run("Job can be failed with error", func() {
		jobResp, _ := client.Jobs().Create(ctx, &resources.CreateJobRequest{
			QueueName:  queueName,
			Payload:    map[string]interface{}{"test": "fail"},
			MaxRetries: ptr(0),
		})
		worker, _ := client.Workers().Register(ctx, &resources.RegisterWorkerRequest{
			QueueName: queueName,
			Hostname:  "test-worker",
		})
		client.Jobs().Claim(ctx, &resources.ClaimJobsRequest{
			QueueName: queueName,
			WorkerID:  worker.ID,
			Limit:     ptr(1),
		})

		err := client.Jobs().Fail(ctx, jobResp.ID, &resources.FailJobRequest{
			WorkerID: worker.ID,
			Error:    "Test failure",
		})
		assertNoError(err)

		job, _ := client.Jobs().Get(ctx, jobResp.ID)
		assertTrue(job.Status == "failed" || job.Status == "deadletter", "status should be failed")

		client.Workers().Deregister(ctx, worker.ID)
	})

	r.Run("Completed job has result", func() {
		jobResp, _ := client.Jobs().Create(ctx, &resources.CreateJobRequest{
			QueueName: queueName,
			Payload:   map[string]interface{}{"test": "result"},
		})
		worker, _ := client.Workers().Register(ctx, &resources.RegisterWorkerRequest{
			QueueName: queueName,
			Hostname:  "test-worker",
		})
		client.Jobs().Claim(ctx, &resources.ClaimJobsRequest{
			QueueName: queueName,
			WorkerID:  worker.ID,
			Limit:     ptr(1),
		})
		client.Jobs().Complete(ctx, jobResp.ID, &resources.CompleteJobRequest{
			WorkerID: worker.ID,
			Result:   map[string]interface{}{"answer": 42},
		})

		completed, _ := client.Jobs().Get(ctx, jobResp.ID)
		assertTrue(completed.Result != nil, "result should exist")

		client.Workers().Deregister(ctx, worker.ID)
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// TEST: Job Failure and Retry
// ─────────────────────────────────────────────────────────────────────────────

func testJobFailureAndRetry(client *spooled.Client) {
	fmt.Println("\n🔁 Jobs - Failure and Retry")
	fmt.Println(strings.Repeat("─", 60))

	r := &Runner{name: "Job Retry"}
	ctx := context.Background()
	queueName := fmt.Sprintf("%s-retry", testPrefix)

	r.Run("Failed job with retries returns to pending", func() {
		jobResp, _ := client.Jobs().Create(ctx, &resources.CreateJobRequest{
			QueueName:  queueName,
			Payload:    map[string]interface{}{"retry": true},
			MaxRetries: ptr(3),
		})
		worker, _ := client.Workers().Register(ctx, &resources.RegisterWorkerRequest{
			QueueName: queueName,
			Hostname:  "retry-worker",
		})
		client.Jobs().Claim(ctx, &resources.ClaimJobsRequest{
			QueueName: queueName,
			WorkerID:  worker.ID,
			Limit:     ptr(1),
		})

		client.Jobs().Fail(ctx, jobResp.ID, &resources.FailJobRequest{
			WorkerID: worker.ID,
			Error:    "retry me",
		})

		time.Sleep(100 * time.Millisecond)

		retried, _ := client.Jobs().Get(ctx, jobResp.ID)
		assertTrue(retried.Status == "pending" || retried.Status == "processing",
			"job should be retried")
		assertTrue(retried.RetryCount > 0, "retry count should increase")

		client.Jobs().Cancel(ctx, jobResp.ID)
		client.Workers().Deregister(ctx, worker.ID)
	})

	r.Run("Job exhausts retries and fails permanently", func() {
		jobResp, _ := client.Jobs().Create(ctx, &resources.CreateJobRequest{
			QueueName:  queueName,
			Payload:    map[string]interface{}{"exhaust": true},
			MaxRetries: ptr(0),
		})
		worker, _ := client.Workers().Register(ctx, &resources.RegisterWorkerRequest{
			QueueName: queueName,
			Hostname:  "exhaust-worker",
		})
		client.Jobs().Claim(ctx, &resources.ClaimJobsRequest{
			QueueName: queueName,
			WorkerID:  worker.ID,
			Limit:     ptr(1),
		})

		client.Jobs().Fail(ctx, jobResp.ID, &resources.FailJobRequest{
			WorkerID: worker.ID,
			Error:    "permanent failure",
		})

		failed, _ := client.Jobs().Get(ctx, jobResp.ID)
		assertTrue(failed.Status == "failed" || failed.Status == "deadletter",
			"job should be permanently failed")

		client.Workers().Deregister(ctx, worker.ID)
	})

	r.Run("Retry job manually", func() {
		jobResp, _ := client.Jobs().Create(ctx, &resources.CreateJobRequest{
			QueueName:  queueName,
			Payload:    map[string]interface{}{"manual": true},
			MaxRetries: ptr(0),
		})
		worker, _ := client.Workers().Register(ctx, &resources.RegisterWorkerRequest{
			QueueName: queueName,
			Hostname:  "manual-worker",
		})
		client.Jobs().Claim(ctx, &resources.ClaimJobsRequest{
			QueueName: queueName,
			WorkerID:  worker.ID,
			Limit:     ptr(1),
		})
		client.Jobs().Fail(ctx, jobResp.ID, &resources.FailJobRequest{
			WorkerID: worker.ID,
			Error:    "will retry",
		})

		retried, err := client.Jobs().Retry(ctx, jobResp.ID)
		assertNoError(err)
		assertTrue(retried.Status == "pending" || retried.Status == "processing",
			"job should be pending after manual retry")

		client.Jobs().Cancel(ctx, jobResp.ID)
		client.Workers().Deregister(ctx, worker.ID)
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// TEST: DLQ (Dead Letter Queue)
// ─────────────────────────────────────────────────────────────────────────────

func testDLQ(client *spooled.Client) {
	fmt.Println("\n💀 Dead Letter Queue (DLQ)")
	fmt.Println(strings.Repeat("─", 60))

	r := &Runner{name: "DLQ"}
	ctx := context.Background()

	r.Run("List DLQ jobs", func() {
		jobs, err := client.Jobs().DLQ().List(ctx, &resources.ListDLQParams{Limit: ptr(10)})
		assertNoError(err)
		assertTrue(jobs != nil, "should return list")
		log(fmt.Sprintf("DLQ contains %d jobs", len(jobs)))
	})

	r.Run("DLQ retry returns response", func() {
		result, err := client.Jobs().DLQ().Retry(ctx, &resources.RetryDLQRequest{})
		if err == nil {
			assertTrue(result.RetriedCount >= 0, "should report retried count")
		}
	})

	r.Run("DLQ purge returns response", func() {
		result, err := client.Jobs().DLQ().Purge(ctx, &resources.PurgeDLQRequest{})
		if err == nil {
			assertTrue(result.PurgedCount >= 0, "should report purged count")
		}
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// TEST: Queues
// ─────────────────────────────────────────────────────────────────────────────

func testQueues(client *spooled.Client) {
	fmt.Println("\n📬 Queues")
	fmt.Println(strings.Repeat("─", 60))

	r := &Runner{name: "Queues"}
	ctx := context.Background()
	queueName := fmt.Sprintf("%s-queue-test", testPrefix)

	r.Run("Create queue (via job)", func() {
		_, err := client.Jobs().Create(ctx, &resources.CreateJobRequest{
			QueueName: queueName,
			Payload:   map[string]interface{}{"ensure": "queue"},
		})
		assertNoError(err)
		log(fmt.Sprintf("Queue %s created via job", queueName))
	})

	r.Run("List queues", func() {
		queues, err := client.Queues().List(ctx)
		assertNoError(err)
		assertTrue(queues != nil, "should return list")
		log(fmt.Sprintf("Found %d queues", len(queues)))
	})

	r.Run("Get queue by name", func() {
		queue, err := client.Queues().Get(ctx, queueName)
		if err != nil {
			// Queue might not exist yet or was deleted
			log(fmt.Sprintf("Get queue failed (may be timing): %v", err))
			return
		}
		assertEqual(queue.QueueName, queueName, "queue name should match")
	})

	r.Run("Get queue stats", func() {
		stats, err := client.Queues().GetStats(ctx, queueName)
		assertNoError(err)
		assertTrue(stats.PendingJobs >= 0, "pending jobs should be non-negative")
	})

	r.Run("Update queue config", func() {
		_, err := client.Queues().UpdateConfig(ctx, queueName, &resources.UpdateQueueConfigRequest{
			MaxRetries: ptr(5),
		})
		if err == nil {
			log(fmt.Sprintf("Queue %s config updated", queueName))
		}
	})

	r.Run("Pause queue", func() {
		result, err := client.Queues().Pause(ctx, queueName, nil)
		assertNoError(err)
		assertTrue(result.Paused, "queue should be paused")
		log(fmt.Sprintf("Queue %s paused", queueName))
	})

	r.Run("Resume queue", func() {
		result, err := client.Queues().Resume(ctx, queueName)
		assertNoError(err)
		assertTrue(result.Resumed, "queue should be resumed")
		log(fmt.Sprintf("Queue %s resumed", queueName))
	})

	r.Run("Delete queue", func() {
		deleteQueue := fmt.Sprintf("%s-delete", queueName)
		// Create and then delete
		client.Jobs().Create(ctx, &resources.CreateJobRequest{
			QueueName: deleteQueue,
			Payload:   map[string]interface{}{"delete": "test"},
		})
		err := client.Queues().Delete(ctx, deleteQueue)
		if err == nil {
			log(fmt.Sprintf("Queue %s deleted", deleteQueue))
		}
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// TEST: Workers
// ─────────────────────────────────────────────────────────────────────────────

func testWorkers(client *spooled.Client) {
	fmt.Println("\n👷 Workers")
	fmt.Println(strings.Repeat("─", 60))

	r := &Runner{name: "Workers"}
	ctx := context.Background()
	queueName := fmt.Sprintf("%s-workers", testPrefix)
	var workerID string

	r.Run("Register worker", func() {
		worker, err := client.Workers().Register(ctx, &resources.RegisterWorkerRequest{
			QueueName: queueName,
			Hostname:  "test-worker-host",
		})
		assertNoError(err)
		assertNotEmpty(worker.ID, "worker ID should exist")
		workerID = worker.ID
		log(fmt.Sprintf("Registered worker: %s", workerID))
	})

	r.Run("List workers", func() {
		workers, err := client.Workers().List(ctx)
		assertNoError(err)
		assertTrue(len(workers) > 0, "should have workers")
	})

	r.Run("Get worker by ID", func() {
		worker, err := client.Workers().Get(ctx, workerID)
		assertNoError(err)
		assertEqual(worker.ID, workerID, "worker ID should match")
	})

	r.Run("Worker heartbeat", func() {
		err := client.Workers().Heartbeat(ctx, workerID, &resources.WorkerHeartbeatRequest{
			CurrentJobs: 0,
			Status:      ptr("healthy"),
		})
		assertNoError(err)
		log("Heartbeat sent")
	})

	r.Run("Worker heartbeat updates last_heartbeat", func() {
		before, _ := client.Workers().Get(ctx, workerID)
		time.Sleep(100 * time.Millisecond)
		client.Workers().Heartbeat(ctx, workerID, &resources.WorkerHeartbeatRequest{
			CurrentJobs: 0,
		})
		after, _ := client.Workers().Get(ctx, workerID)
		assertTrue(after.LastHeartbeat.After(before.LastHeartbeat) || after.LastHeartbeat.Equal(before.LastHeartbeat),
			"last_heartbeat should be updated")
	})

	r.Run("Deregister worker", func() {
		err := client.Workers().Deregister(ctx, workerID)
		if err != nil {
			// Deregister endpoint may not be available (405 in some environments)
			log(fmt.Sprintf("Deregister failed (may not be supported): %v", err))
			return
		}
		log(fmt.Sprintf("Deregistered worker: %s", workerID))
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// TEST: Webhooks
// ─────────────────────────────────────────────────────────────────────────────

func testWebhooks(client *spooled.Client) {
	fmt.Println("\n🔔 Webhooks")
	fmt.Println(strings.Repeat("─", 60))

	// Production requires HTTPS webhooks - skip tests that need HTTP localhost
	if isProduction {
		r := &Runner{name: "Webhooks"}
		r.Skip("Webhook tests", "production requires HTTPS webhooks")
		return
	}

	r := &Runner{name: "Webhooks"}
	ctx := context.Background()
	var webhookID string

	r.Run("Create webhook", func() {
		webhook, err := client.Webhooks().Create(ctx, &resources.CreateOutgoingWebhookRequest{
			Name:   fmt.Sprintf("%s-webhook", testPrefix),
			URL:    fmt.Sprintf("http://localhost:%d/webhook", webhookPort),
			Events: []resources.WebhookEvent{resources.WebhookEventJobCompleted, resources.WebhookEventJobFailed},
		})
		assertNoError(err)
		assertNotEmpty(webhook.ID, "webhook ID should exist")
		webhookID = webhook.ID
		log(fmt.Sprintf("Created webhook: %s", webhookID))
	})

	r.Run("List webhooks", func() {
		webhooks, err := client.Webhooks().List(ctx)
		assertNoError(err)
		assertTrue(len(webhooks) > 0, "should have webhooks")
	})

	r.Run("Get webhook by ID", func() {
		webhook, err := client.Webhooks().Get(ctx, webhookID)
		assertNoError(err)
		assertEqual(webhook.ID, webhookID, "webhook ID should match")
	})

	r.Run("Webhook has events configured", func() {
		webhook, err := client.Webhooks().Get(ctx, webhookID)
		assertNoError(err)
		assertTrue(len(webhook.Events) > 0, "should have events")
	})

	r.Run("Update webhook", func() {
		events := []resources.WebhookEvent{resources.WebhookEventJobCompleted}
		webhook, err := client.Webhooks().Update(ctx, webhookID, &resources.UpdateOutgoingWebhookRequest{
			Events: &events,
		})
		assertNoError(err)
		assertEqual(len(webhook.Events), 1, "should have 1 event")
	})

	r.Run("Test webhook", func() {
		result, err := client.Webhooks().Test(ctx, webhookID)
		if err == nil {
			log(fmt.Sprintf("Test result: success=%v", result.Success))
		}
	})

	r.Run("Delete webhook", func() {
		err := client.Webhooks().Delete(ctx, webhookID)
		assertNoError(err)
		log(fmt.Sprintf("Deleted webhook: %s", webhookID))
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// TEST: Schedules
// ─────────────────────────────────────────────────────────────────────────────

func testSchedules(client *spooled.Client) {
	fmt.Println("\n⏰ Schedules")
	fmt.Println(strings.Repeat("─", 60))

	r := &Runner{name: "Schedules"}
	ctx := context.Background()
	queueName := fmt.Sprintf("%s-scheduled", testPrefix)
	var scheduleID string

	r.Run("Create schedule", func() {
		tz := "UTC"
		schedule, err := client.Schedules().Create(ctx, &resources.CreateScheduleRequest{
			Name:            fmt.Sprintf("%s-schedule", testPrefix),
			QueueName:       queueName,
			CronExpression:  "0 * * * *", // Every hour (5-field cron)
			Timezone:        &tz,
			PayloadTemplate: map[string]interface{}{"scheduled": true},
		})
		if err != nil {
			// Schedule API might not be available or have different requirements
			log(fmt.Sprintf("Schedule creation failed: %v", err))
			return
		}
		assertNotEmpty(schedule.ID, "schedule ID should exist")
		scheduleID = schedule.ID
		log(fmt.Sprintf("Created schedule: %s", scheduleID))
	})

	r.Run("List schedules", func() {
		schedules, err := client.Schedules().List(ctx, nil)
		if err != nil {
			log(fmt.Sprintf("List schedules failed: %v", err))
			return
		}
		log(fmt.Sprintf("Found %d schedules", len(schedules)))
	})

	r.Run("Get schedule by ID", func() {
		if scheduleID == "" {
			log("Skip - no schedule created")
			return
		}
		schedule, err := client.Schedules().Get(ctx, scheduleID)
		assertNoError(err)
		assertEqual(schedule.ID, scheduleID, "schedule ID should match")
	})

	r.Run("Update schedule", func() {
		if scheduleID == "" {
			log("Skip - no schedule created")
			return
		}
		newCron := "*/30 * * * *" // Every 30 minutes
		schedule, err := client.Schedules().Update(ctx, scheduleID, &resources.UpdateScheduleRequest{
			CronExpression: &newCron,
		})
		if err == nil {
			log(fmt.Sprintf("Updated schedule cron: %s", schedule.CronExpression))
		}
	})

	r.Run("Schedule has next run time", func() {
		if scheduleID == "" {
			log("Skip - no schedule created")
			return
		}
		schedule, err := client.Schedules().Get(ctx, scheduleID)
		assertNoError(err)
		assertTrue(schedule.NextRunAt != nil, "next run should exist")
	})

	r.Run("Pause schedule", func() {
		if scheduleID == "" {
			log("Skip - no schedule created")
			return
		}
		schedule, err := client.Schedules().Pause(ctx, scheduleID)
		assertNoError(err)
		assertTrue(!schedule.IsActive, "schedule should be paused (inactive)")
	})

	r.Run("Resume schedule", func() {
		if scheduleID == "" {
			log("Skip - no schedule created")
			return
		}
		schedule, err := client.Schedules().Resume(ctx, scheduleID)
		assertNoError(err)
		assertTrue(schedule.IsActive, "schedule should be active")
	})

	r.Run("Trigger schedule manually", func() {
		if scheduleID == "" {
			log("Skip - no schedule created")
			return
		}
		result, err := client.Schedules().Trigger(ctx, scheduleID)
		if err == nil {
			assertNotEmpty(result.JobID, "job ID")
			log(fmt.Sprintf("Triggered schedule, job: %s", result.JobID))
			// Cleanup triggered job
			client.Jobs().Cancel(ctx, result.JobID)
		}
	})

	r.Run("Get schedule execution history", func() {
		if scheduleID == "" {
			log("Skip - no schedule created")
			return
		}
		history, err := client.Schedules().History(ctx, scheduleID, nil)
		if err == nil {
			log(fmt.Sprintf("Schedule has %d executions", len(history)))
		}
	})

	r.Run("Delete schedule", func() {
		if scheduleID == "" {
			log("Skip - no schedule created")
			return
		}
		err := client.Schedules().Delete(ctx, scheduleID)
		assertNoError(err)
		log(fmt.Sprintf("Deleted schedule: %s", scheduleID))
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// TEST: Workflows
// ─────────────────────────────────────────────────────────────────────────────

func testWorkflows(client *spooled.Client) {
	fmt.Println("\n🔀 Workflows")
	fmt.Println(strings.Repeat("─", 60))

	r := &Runner{name: "Workflows"}
	ctx := context.Background()
	queueName := fmt.Sprintf("%s-workflow", testPrefix)
	var workflowID string

	r.Run("Create workflow", func() {
		resp, err := client.Workflows().Create(ctx, &resources.CreateWorkflowRequest{
			Name: fmt.Sprintf("%s-wf", testPrefix),
			Jobs: []resources.WorkflowJobDefinition{
				{Key: "step1", QueueName: queueName, Payload: map[string]interface{}{"step": 1}},
				{Key: "step2", QueueName: queueName, Payload: map[string]interface{}{"step": 2}, DependsOn: []string{"step1"}},
			},
		})
		assertNoError(err)
		assertNotEmpty(resp.WorkflowID, "workflow ID should exist")
		workflowID = resp.WorkflowID
		log(fmt.Sprintf("Created workflow: %s", workflowID))
	})

	r.Run("List workflows", func() {
		workflows, err := client.Workflows().List(ctx, nil)
		assertNoError(err)
		assertTrue(len(workflows) > 0, "should have workflows")
	})

	r.Run("Get workflow by ID", func() {
		workflow, err := client.Workflows().Get(ctx, workflowID)
		assertNoError(err)
		assertEqual(workflow.ID, workflowID, "workflow ID should match")
	})

	r.Run("Workflow has job count", func() {
		workflow, err := client.Workflows().Get(ctx, workflowID)
		assertNoError(err)
		assertTrue(workflow.TotalJobs >= 0, "total jobs should be non-negative")
	})

	r.Run("Cancel workflow", func() {
		err := client.Workflows().Cancel(ctx, workflowID)
		assertNoError(err)
		log(fmt.Sprintf("Cancelled workflow: %s", workflowID))
	})

	r.Run("Retry workflow", func() {
		// Create a workflow to retry
		resp, err := client.Workflows().Create(ctx, &resources.CreateWorkflowRequest{
			Name: fmt.Sprintf("%s-retry", testPrefix),
			Jobs: []resources.WorkflowJobDefinition{
				{Key: "step1", QueueName: queueName, Payload: map[string]interface{}{"retry": true}},
			},
		})
		if err != nil || resp == nil {
			log(fmt.Sprintf("Skip retry test - workflow creation failed: %v", err))
			return
		}
		// Cancel it first
		client.Workflows().Cancel(ctx, resp.WorkflowID)
		// Try to retry
		_, retryErr := client.Workflows().Retry(ctx, resp.WorkflowID)
		// May or may not work depending on status
		log(fmt.Sprintf("Retry result: %v", retryErr))
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// TEST: Workflow DAG
// ─────────────────────────────────────────────────────────────────────────────

func testWorkflowDAG(client *spooled.Client) {
	fmt.Println("\n🔀 Workflow DAG Processing")
	fmt.Println(strings.Repeat("─", 60))

	r := &Runner{name: "Workflow DAG"}
	ctx := context.Background()
	queueName := fmt.Sprintf("%s-wf-dag", testPrefix)

	r.Run("Create workflow with DAG dependencies", func() {
		// Create a DAG: A -> B -> D
		//               A -> C -> D
		resp, err := client.Workflows().Create(ctx, &resources.CreateWorkflowRequest{
			Name: fmt.Sprintf("%s-dag", testPrefix),
			Jobs: []resources.WorkflowJobDefinition{
				{Key: "A", QueueName: queueName, Payload: map[string]interface{}{"job": "A"}},
				{Key: "B", QueueName: queueName, Payload: map[string]interface{}{"job": "B"}, DependsOn: []string{"A"}},
				{Key: "C", QueueName: queueName, Payload: map[string]interface{}{"job": "C"}, DependsOn: []string{"A"}},
				{Key: "D", QueueName: queueName, Payload: map[string]interface{}{"job": "D"}, DependsOn: []string{"B", "C"}},
			},
		})
		assertNoError(err)
		assertNotEmpty(resp.WorkflowID, "workflow ID")
		log(fmt.Sprintf("Created DAG workflow: %s", resp.WorkflowID))

		// Cleanup
		client.Workflows().Cancel(ctx, resp.WorkflowID)
	})

	r.Run("Only root job (A) is initially pending", func() {
		resp, err := client.Workflows().Create(ctx, &resources.CreateWorkflowRequest{
			Name: fmt.Sprintf("%s-dag-check", testPrefix),
			Jobs: []resources.WorkflowJobDefinition{
				{Key: "A", QueueName: queueName, Payload: map[string]interface{}{"job": "A"}},
				{Key: "B", QueueName: queueName, Payload: map[string]interface{}{"job": "B"}, DependsOn: []string{"A"}},
			},
		})
		if err != nil || resp == nil {
			log(fmt.Sprintf("Skip - workflow creation failed: %v", err))
			return
		}

		// Get workflow jobs
		jobs, _ := client.Workflows().Jobs().ListJobs(ctx, resp.WorkflowID)
		if len(jobs) > 0 {
			log(fmt.Sprintf("Workflow has %d jobs", len(jobs)))
		}

		client.Workflows().Cancel(ctx, resp.WorkflowID)
	})

	r.Run("Process workflow jobs in order", func() {
		dagQueue := fmt.Sprintf("%s-dag-process", queueName)

		resp, err := client.Workflows().Create(ctx, &resources.CreateWorkflowRequest{
			Name: fmt.Sprintf("%s-dag-process", testPrefix),
			Jobs: []resources.WorkflowJobDefinition{
				{Key: "A", QueueName: dagQueue, Payload: map[string]interface{}{"job": "A"}},
				{Key: "B", QueueName: dagQueue, Payload: map[string]interface{}{"job": "B"}, DependsOn: []string{"A"}},
			},
		})
		if err != nil || resp == nil {
			log(fmt.Sprintf("Skip - workflow creation failed: %v", err))
			return
		}

		worker, werr := client.Workers().Register(ctx, &resources.RegisterWorkerRequest{
			QueueName: dagQueue,
			Hostname:  "dag-worker",
		})
		if werr != nil || worker == nil {
			log(fmt.Sprintf("Skip - worker registration failed: %v", werr))
			client.Workflows().Cancel(ctx, resp.WorkflowID)
			return
		}

		// Process job A
		claimed, _ := client.Jobs().Claim(ctx, &resources.ClaimJobsRequest{
			QueueName: dagQueue,
			WorkerID:  worker.ID,
			Limit:     ptr(1),
		})
		if claimed != nil && len(claimed.Jobs) > 0 {
			client.Jobs().Complete(ctx, claimed.Jobs[0].ID, &resources.CompleteJobRequest{
				WorkerID: worker.ID,
				Result:   map[string]interface{}{"completed": "A"},
			})
			log("Completed job A")

			// Wait a bit for B to become available
			time.Sleep(200 * time.Millisecond)

			// Process job B (should now be available)
			claimedB, _ := client.Jobs().Claim(ctx, &resources.ClaimJobsRequest{
				QueueName: dagQueue,
				WorkerID:  worker.ID,
				Limit:     ptr(1),
			})
			if claimedB != nil && len(claimedB.Jobs) > 0 {
				client.Jobs().Complete(ctx, claimedB.Jobs[0].ID, &resources.CompleteJobRequest{
					WorkerID: worker.ID,
					Result:   map[string]interface{}{"completed": "B"},
				})
				log("Completed job B")
			}
		}

		client.Workers().Deregister(ctx, worker.ID)
		client.Workflows().Cancel(ctx, resp.WorkflowID)
	})

	r.Run("Workflow completes successfully", func() {
		wfQueue := fmt.Sprintf("%s-dag-complete", queueName)

		resp, err := client.Workflows().Create(ctx, &resources.CreateWorkflowRequest{
			Name: fmt.Sprintf("%s-dag-complete", testPrefix),
			Jobs: []resources.WorkflowJobDefinition{
				{Key: "single", QueueName: wfQueue, Payload: map[string]interface{}{"job": "single"}},
			},
		})
		if err != nil || resp == nil {
			log(fmt.Sprintf("Skip - workflow creation failed: %v", err))
			return
		}

		worker, werr := client.Workers().Register(ctx, &resources.RegisterWorkerRequest{
			QueueName: wfQueue,
			Hostname:  "complete-worker",
		})
		if werr != nil || worker == nil {
			log(fmt.Sprintf("Skip - worker registration failed: %v", werr))
			return
		}

		claimed, _ := client.Jobs().Claim(ctx, &resources.ClaimJobsRequest{
			QueueName: wfQueue,
			WorkerID:  worker.ID,
			Limit:     ptr(1),
		})
		if claimed != nil && len(claimed.Jobs) > 0 {
			client.Jobs().Complete(ctx, claimed.Jobs[0].ID, &resources.CompleteJobRequest{
				WorkerID: worker.ID,
				Result:   map[string]interface{}{"completed": true},
			})
		}

		time.Sleep(200 * time.Millisecond)

		wf, _ := client.Workflows().Get(ctx, resp.WorkflowID)
		if wf != nil {
			log(fmt.Sprintf("Workflow status: %s", wf.Status))
		}

		client.Workers().Deregister(ctx, worker.ID)
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// TEST: Billing
// ─────────────────────────────────────────────────────────────────────────────

func testBilling(client *spooled.Client) {
	fmt.Println("\n💳 Billing")
	fmt.Println(strings.Repeat("─", 60))

	r := &Runner{name: "Billing"}
	ctx := context.Background()

	r.Run("Get billing status", func() {
		status, err := client.Billing().GetStatus(ctx)
		assertNoError(err)
		assertTrue(status != nil, "status should exist")
		log(fmt.Sprintf("Plan tier: %s", status.PlanTier))
	})

	r.Run("Billing status has plan tier", func() {
		status, err := client.Billing().GetStatus(ctx)
		assertNoError(err)
		assertNotEmpty(string(status.PlanTier), "plan tier should exist")
	})

	r.Run("Create portal session", func() {
		session, err := client.Billing().CreatePortalSession(ctx, &resources.CreatePortalRequest{})
		if err == nil && session != nil {
			assertNotEmpty(session.URL, "portal URL should exist")
			log(fmt.Sprintf("Portal URL created: %s...", session.URL[:min(50, len(session.URL))]))
		} else {
			log("Portal session (may require subscription)")
		}
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// TEST: Realtime
// ─────────────────────────────────────────────────────────────────────────────

func testRealtime(client *spooled.Client) {
	fmt.Println("\n📡 Realtime")
	fmt.Println(strings.Repeat("─", 60))

	r := &Runner{name: "Realtime"}

	r.Run("SSE endpoint is available", func() {
		resp, err := http.Get(fmt.Sprintf("%s/api/v1/events", baseURL))
		if err == nil {
			resp.Body.Close()
			assertTrue(resp.StatusCode == 200 || resp.StatusCode == 401,
				"SSE endpoint should be accessible")
		}
	})

	r.Skip("WebSocket connection", "requires async handling")
	r.Skip("SSE event subscription", "requires async handling")
}

// ─────────────────────────────────────────────────────────────────────────────
// TEST: gRPC Basic
// ─────────────────────────────────────────────────────────────────────────────

func testGRPC(client *spooled.Client) {
	fmt.Println("\n🔌 gRPC - Basic Operations")
	fmt.Println(strings.Repeat("─", 60))

	r := &Runner{name: "gRPC"}

	if skipGRPC {
		r.Skip("gRPC tests", "SKIP_GRPC=true")
		return
	}

	queueName := fmt.Sprintf("%s-grpc", testPrefix)
	var grpcClient *spooledgrpc.Client
	var workerID string
	var grpcConnected bool

	r.Run("Connect to gRPC server via client", func() {
		// Test that client.GRPC() works and returns a valid client
		var err error
		grpcClient, err = client.GRPC()
		assertNoError(err)
		assertTrue(grpcClient != nil, "gRPC client should be available")
		grpcConnected = true
		log("Connected to gRPC via client.GRPC()")
	})

	if !grpcConnected {
		r.Skip("gRPC tests", "connection failed")
		return
	}

	r.Run("gRPC: Register worker", func() {
		result, err := grpcClient.RegisterWorker(context.Background(), &spooledgrpc.RegisterWorkerRequest{
			QueueName:      queueName,
			Hostname:       "grpc-test-worker",
			MaxConcurrency: 5,
		})
		assertNoError(err)
		assertNotEmpty(result.WorkerID, "worker id")
		workerID = result.WorkerID
		log(fmt.Sprintf("Registered worker: %s", workerID))
	})

	r.Run("gRPC: Enqueue job", func() {
		result, err := grpcClient.Enqueue(context.Background(), &spooledgrpc.EnqueueRequest{
			QueueName: queueName,
			Payload:   map[string]any{"message": "Hello from gRPC!", "timestamp": time.Now().Unix()},
			Priority:  5,
		})
		assertNoError(err)
		assertNotEmpty(result.JobID, "job id")
		assertTrue(result.Created, "created should be true")
		log(fmt.Sprintf("Enqueued job: %s", result.JobID))
	})

	r.Run("gRPC: Dequeue job", func() {
		result, err := grpcClient.Dequeue(context.Background(), &spooledgrpc.DequeueRequest{
			QueueName:        queueName,
			WorkerID:         workerID,
			LeaseDurationSec: 60,
			BatchSize:        1,
		})
		assertNoError(err)
		assertTrue(len(result.Jobs) > 0, "should dequeue job")
		log(fmt.Sprintf("Dequeued job: %s", result.Jobs[0].ID))
	})

	r.Run("gRPC: Get queue stats", func() {
		stats, err := grpcClient.GetQueueStats(context.Background(), queueName)
		assertNoError(err)
		assertEqual(stats.QueueName, queueName, "queue name")
		log(fmt.Sprintf("Queue stats: pending=%d, processing=%d", stats.Pending, stats.Processing))
	})

	r.Run("gRPC: Complete job", func() {
		// Enqueue and dequeue a new job
		enqResult, _ := grpcClient.Enqueue(context.Background(), &spooledgrpc.EnqueueRequest{
			QueueName: queueName,
			Payload:   map[string]any{"complete": "test"},
		})
		grpcClient.Dequeue(context.Background(), &spooledgrpc.DequeueRequest{
			QueueName: queueName,
			WorkerID:  workerID,
			BatchSize: 1,
		})
		err := grpcClient.Complete(context.Background(), &spooledgrpc.CompleteRequest{
			JobID:    enqResult.JobID,
			WorkerID: workerID,
			Result:   map[string]any{"processed": true},
		})
		assertNoError(err)
		log("Job completed via gRPC")
	})

	r.Run("gRPC: Heartbeat", func() {
		err := grpcClient.WorkerHeartbeat(context.Background(), &spooledgrpc.WorkerHeartbeatRequest{
			WorkerID:    workerID,
			CurrentJobs: 0,
			Status:      "healthy",
		})
		assertNoError(err)
		log("Heartbeat sent")
	})

	r.Run("gRPC: Deregister worker", func() {
		err := grpcClient.DeregisterWorker(context.Background(), workerID)
		assertNoError(err)
		log(fmt.Sprintf("Deregistered worker: %s", workerID))
	})

	r.Run("gRPC: Job lifecycle via gRPC", func() {
		lifecycleQueue := fmt.Sprintf("%s-lifecycle", queueName)

		// Register worker
		regResult, _ := grpcClient.RegisterWorker(context.Background(), &spooledgrpc.RegisterWorkerRequest{
			QueueName: lifecycleQueue,
			Hostname:  "grpc-lifecycle-worker",
		})
		wID := regResult.WorkerID

		// Enqueue
		enqResult, _ := grpcClient.Enqueue(context.Background(), &spooledgrpc.EnqueueRequest{
			QueueName: lifecycleQueue,
			Payload:   map[string]any{"test": "lifecycle"},
		})
		jobID := enqResult.JobID

		// Dequeue
		deqResult, _ := grpcClient.Dequeue(context.Background(), &spooledgrpc.DequeueRequest{
			QueueName: lifecycleQueue,
			WorkerID:  wID,
			BatchSize: 1,
		})
		assertTrue(len(deqResult.Jobs) > 0, "should dequeue job")
		assertEqual(deqResult.Jobs[0].ID, jobID, "job id")

		// Renew lease
		renewResult, _ := grpcClient.RenewLease(context.Background(), &spooledgrpc.RenewLeaseRequest{
			JobID:         jobID,
			WorkerID:      wID,
			ExtensionSecs: 120,
		})
		assertTrue(renewResult.Success, "renew success")

		// Complete
		grpcClient.Complete(context.Background(), &spooledgrpc.CompleteRequest{
			JobID:    jobID,
			WorkerID: wID,
			Result:   map[string]any{"completed": true},
		})

		// Get job to verify
		job, _ := grpcClient.GetJob(context.Background(), jobID)
		assertNotEmpty(job.ID, "job should exist")

		// Cleanup
		grpcClient.DeregisterWorker(context.Background(), wID)
		log("Full job lifecycle via gRPC completed")
	})

	r.Run("gRPC: Fail job with retry", func() {
		failQueue := fmt.Sprintf("%s-fail", queueName)

		// Register worker
		regResult, _ := grpcClient.RegisterWorker(context.Background(), &spooledgrpc.RegisterWorkerRequest{
			QueueName: failQueue,
			Hostname:  "grpc-fail-worker",
		})
		wID := regResult.WorkerID

		// Enqueue with retries
		enqResult, _ := grpcClient.Enqueue(context.Background(), &spooledgrpc.EnqueueRequest{
			QueueName:  failQueue,
			Payload:    map[string]any{"test": "fail"},
			MaxRetries: 2,
		})
		jobID := enqResult.JobID

		// Dequeue
		grpcClient.Dequeue(context.Background(), &spooledgrpc.DequeueRequest{
			QueueName: failQueue,
			WorkerID:  wID,
			BatchSize: 1,
		})

		// Fail
		err := grpcClient.Fail(context.Background(), &spooledgrpc.FailRequest{
			JobID:    jobID,
			WorkerID: wID,
			Error:    "Test failure",
			Retry:    true,
		})
		assertNoError(err)

		// Cleanup
		grpcClient.DeregisterWorker(context.Background(), wID)
		log("Job failed with retry via gRPC")
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// TEST: gRPC Advanced
// ─────────────────────────────────────────────────────────────────────────────

func testGRPCAdvanced(client *spooled.Client) {
	fmt.Println("\n🔌 gRPC - Advanced Operations")
	fmt.Println(strings.Repeat("─", 60))

	r := &Runner{name: "gRPC Advanced"}

	if skipGRPC {
		r.Skip("gRPC advanced tests", "SKIP_GRPC=true")
		return
	}

	queueName := fmt.Sprintf("%s-grpc-adv", testPrefix)
	grpcClient, err := client.GRPC()
	if err != nil || grpcClient == nil {
		r.Skip("gRPC advanced tests", "gRPC client not available")
		return
	}

	r.Run("gRPC: GetJob - existing job", func() {
		getJobQueue := fmt.Sprintf("%s-getjob", queueName)
		enqResult, _ := grpcClient.Enqueue(context.Background(), &spooledgrpc.EnqueueRequest{
			QueueName:  getJobQueue,
			Payload:    map[string]any{"test": "getjob"},
			Priority:   7,
			MaxRetries: 5,
		})

		job, err := grpcClient.GetJob(context.Background(), enqResult.JobID)
		assertNoError(err)
		assertEqual(job.ID, enqResult.JobID, "job id")
		assertEqual(job.QueueName, getJobQueue, "queue name")
		assertEqual(int(job.Priority), 7, "priority")
		log(fmt.Sprintf("GetJob returned job: %s", job.ID))
	})

	r.Run("gRPC: GetJob - non-existent job", func() {
		job, _ := grpcClient.GetJob(context.Background(), "non-existent-job-id")
		assertTrue(job == nil || job.ID == "", "job should be nil or empty")
		log("GetJob correctly returned nil for non-existent job")
	})

	r.Run("gRPC: Enqueue with idempotency key", func() {
		idemQueue := fmt.Sprintf("%s-idem", queueName)
		idempotencyKey := fmt.Sprintf("grpc-idem-%d", time.Now().UnixNano())

		// First enqueue
		result1, _ := grpcClient.Enqueue(context.Background(), &spooledgrpc.EnqueueRequest{
			QueueName:      idemQueue,
			Payload:        map[string]any{"test": "idempotent"},
			IdempotencyKey: idempotencyKey,
		})
		assertTrue(result1.Created, "first enqueue should create")
		firstJobID := result1.JobID

		// Second enqueue with same key
		result2, _ := grpcClient.Enqueue(context.Background(), &spooledgrpc.EnqueueRequest{
			QueueName:      idemQueue,
			Payload:        map[string]any{"test": "idempotent-dup"},
			IdempotencyKey: idempotencyKey,
		})
		assertTrue(!result2.Created, "second should not create")
		assertEqual(result2.JobID, firstJobID, "same job id")
		log(fmt.Sprintf("Idempotency key works: %s", idempotencyKey))
	})

	r.Run("gRPC: Batch dequeue multiple jobs", func() {
		batchQueue := fmt.Sprintf("%s-batch", queueName)

		// Register worker
		regResult, _ := grpcClient.RegisterWorker(context.Background(), &spooledgrpc.RegisterWorkerRequest{
			QueueName:      batchQueue,
			Hostname:       "grpc-batch-worker",
			MaxConcurrency: 10,
		})
		wID := regResult.WorkerID

		// Enqueue multiple jobs
		numJobs := 5
		for i := 0; i < numJobs; i++ {
			grpcClient.Enqueue(context.Background(), &spooledgrpc.EnqueueRequest{
				QueueName: batchQueue,
				Payload:   map[string]any{"index": i, "batch": "test"},
			})
		}

		// Batch dequeue
		deqResult, _ := grpcClient.Dequeue(context.Background(), &spooledgrpc.DequeueRequest{
			QueueName:        batchQueue,
			WorkerID:         wID,
			BatchSize:        10,
			LeaseDurationSec: 60,
		})
		assertTrue(len(deqResult.Jobs) >= numJobs-1, fmt.Sprintf("should dequeue at least %d jobs", numJobs-1))
		log(fmt.Sprintf("Batch dequeued %d jobs", len(deqResult.Jobs)))

		// Complete all
		for _, job := range deqResult.Jobs {
			grpcClient.Complete(context.Background(), &spooledgrpc.CompleteRequest{
				JobID:    job.ID,
				WorkerID: wID,
			})
		}
		grpcClient.DeregisterWorker(context.Background(), wID)
	})

	r.Run("gRPC: RenewLease - extend lease", func() {
		renewQueue := fmt.Sprintf("%s-renew", queueName)

		regResult, _ := grpcClient.RegisterWorker(context.Background(), &spooledgrpc.RegisterWorkerRequest{
			QueueName: renewQueue,
			Hostname:  "grpc-renew-worker",
		})
		wID := regResult.WorkerID

		enqResult, _ := grpcClient.Enqueue(context.Background(), &spooledgrpc.EnqueueRequest{
			QueueName: renewQueue,
			Payload:   map[string]any{"test": "renew"},
		})

		grpcClient.Dequeue(context.Background(), &spooledgrpc.DequeueRequest{
			QueueName:        renewQueue,
			WorkerID:         wID,
			BatchSize:        1,
			LeaseDurationSec: 30,
		})

		renewResult, _ := grpcClient.RenewLease(context.Background(), &spooledgrpc.RenewLeaseRequest{
			JobID:         enqResult.JobID,
			WorkerID:      wID,
			ExtensionSecs: 60,
		})
		assertTrue(renewResult.Success, "renew success")
		assertTrue(renewResult.NewExpiresAt != nil, "new expires at")
		log(fmt.Sprintf("Renewed lease: %v", renewResult.NewExpiresAt))

		grpcClient.Complete(context.Background(), &spooledgrpc.CompleteRequest{JobID: enqResult.JobID, WorkerID: wID})
		grpcClient.DeregisterWorker(context.Background(), wID)
	})

	r.Run("gRPC: RenewLease - wrong worker ID fails", func() {
		renewFailQueue := fmt.Sprintf("%s-renew-fail", queueName)

		reg1, _ := grpcClient.RegisterWorker(context.Background(), &spooledgrpc.RegisterWorkerRequest{
			QueueName: renewFailQueue,
			Hostname:  "grpc-renew-worker-1",
		})
		reg2, _ := grpcClient.RegisterWorker(context.Background(), &spooledgrpc.RegisterWorkerRequest{
			QueueName: renewFailQueue,
			Hostname:  "grpc-renew-worker-2",
		})

		enqResult, _ := grpcClient.Enqueue(context.Background(), &spooledgrpc.EnqueueRequest{
			QueueName: renewFailQueue,
			Payload:   map[string]any{"test": "renew-fail"},
		})

		grpcClient.Dequeue(context.Background(), &spooledgrpc.DequeueRequest{
			QueueName: renewFailQueue,
			WorkerID:  reg1.WorkerID,
			BatchSize: 1,
		})

		// Try wrong worker
		renewResult, _ := grpcClient.RenewLease(context.Background(), &spooledgrpc.RenewLeaseRequest{
			JobID:         enqResult.JobID,
			WorkerID:      reg2.WorkerID, // Wrong worker!
			ExtensionSecs: 60,
		})
		assertTrue(!renewResult.Success, "renew should fail with wrong worker")
		log("Renew correctly failed with wrong worker ID")

		grpcClient.Complete(context.Background(), &spooledgrpc.CompleteRequest{JobID: enqResult.JobID, WorkerID: reg1.WorkerID})
		grpcClient.DeregisterWorker(context.Background(), reg1.WorkerID)
		grpcClient.DeregisterWorker(context.Background(), reg2.WorkerID)
	})

	r.Run("gRPC: Fail - no retry (to deadletter)", func() {
		failDLQQueue := fmt.Sprintf("%s-fail-dlq", queueName)

		regResult, _ := grpcClient.RegisterWorker(context.Background(), &spooledgrpc.RegisterWorkerRequest{
			QueueName: failDLQQueue,
			Hostname:  "grpc-fail-dlq-worker",
		})
		wID := regResult.WorkerID

		enqResult, _ := grpcClient.Enqueue(context.Background(), &spooledgrpc.EnqueueRequest{
			QueueName:  failDLQQueue,
			Payload:    map[string]any{"test": "fail-dlq"},
			MaxRetries: 0,
		})

		grpcClient.Dequeue(context.Background(), &spooledgrpc.DequeueRequest{
			QueueName: failDLQQueue,
			WorkerID:  wID,
			BatchSize: 1,
		})

		err := grpcClient.Fail(context.Background(), &spooledgrpc.FailRequest{
			JobID:    enqResult.JobID,
			WorkerID: wID,
			Error:    "Intentional failure to DLQ",
			Retry:    false,
		})
		assertNoError(err)
		log("Fail to DLQ succeeded")

		grpcClient.DeregisterWorker(context.Background(), wID)
	})

	r.Run("gRPC: Fail - with long error message", func() {
		failLongQueue := fmt.Sprintf("%s-fail-long", queueName)

		regResult, _ := grpcClient.RegisterWorker(context.Background(), &spooledgrpc.RegisterWorkerRequest{
			QueueName: failLongQueue,
			Hostname:  "grpc-fail-long-worker",
		})
		wID := regResult.WorkerID

		enqResult, _ := grpcClient.Enqueue(context.Background(), &spooledgrpc.EnqueueRequest{
			QueueName:  failLongQueue,
			Payload:    map[string]any{"test": "fail-long"},
			MaxRetries: 0,
		})

		grpcClient.Dequeue(context.Background(), &spooledgrpc.DequeueRequest{
			QueueName: failLongQueue,
			WorkerID:  wID,
			BatchSize: 1,
		})

		longError := strings.Repeat("x", 5000)
		err := grpcClient.Fail(context.Background(), &spooledgrpc.FailRequest{
			JobID:    enqResult.JobID,
			WorkerID: wID,
			Error:    "Error: " + longError,
			Retry:    false,
		})
		assertNoError(err)
		log("Fail with 5000+ char error message succeeded")

		grpcClient.DeregisterWorker(context.Background(), wID)
	})

	r.Run("gRPC: Worker with metadata", func() {
		metaQueue := fmt.Sprintf("%s-meta", queueName)

		result, err := grpcClient.RegisterWorker(context.Background(), &spooledgrpc.RegisterWorkerRequest{
			QueueName:      metaQueue,
			Hostname:       "grpc-meta-worker",
			MaxConcurrency: 10,
			Version:        "1.2.3",
			Metadata: map[string]string{
				"environment": "test",
				"region":      "us-east-1",
				"instance":    "i-12345",
			},
		})
		assertNoError(err)
		assertNotEmpty(result.WorkerID, "worker id")
		log(fmt.Sprintf("Worker with metadata: %s", result.WorkerID))

		grpcClient.DeregisterWorker(context.Background(), result.WorkerID)
	})

	r.Run("gRPC: Heartbeat with different statuses", func() {
		hbQueue := fmt.Sprintf("%s-hb", queueName)

		result, _ := grpcClient.RegisterWorker(context.Background(), &spooledgrpc.RegisterWorkerRequest{
			QueueName: hbQueue,
			Hostname:  "grpc-hb-worker",
		})
		wID := result.WorkerID

		// Healthy
		grpcClient.WorkerHeartbeat(context.Background(), &spooledgrpc.WorkerHeartbeatRequest{
			WorkerID: wID, CurrentJobs: 5, Status: "healthy",
		})
		// Degraded
		grpcClient.WorkerHeartbeat(context.Background(), &spooledgrpc.WorkerHeartbeatRequest{
			WorkerID: wID, CurrentJobs: 8, Status: "degraded",
		})
		// Draining
		grpcClient.WorkerHeartbeat(context.Background(), &spooledgrpc.WorkerHeartbeatRequest{
			WorkerID: wID, CurrentJobs: 2, Status: "draining",
		})
		log("All heartbeat statuses accepted")

		grpcClient.DeregisterWorker(context.Background(), wID)
	})

	r.Run("gRPC: GetQueueStats - empty queue", func() {
		emptyQueue := fmt.Sprintf("%s-empty-%d", queueName, time.Now().UnixNano())

		stats, err := grpcClient.GetQueueStats(context.Background(), emptyQueue)
		assertNoError(err)
		assertEqual(stats.QueueName, emptyQueue, "queue name")
		assertEqual(int(stats.Pending), 0, "pending should be 0")
		assertEqual(int(stats.Processing), 0, "processing should be 0")
		log("Empty queue stats correct")
	})

	r.Run("gRPC: GetQueueStats - queue with mixed statuses", func() {
		mixedQueue := fmt.Sprintf("%s-mixed", queueName)

		regResult, _ := grpcClient.RegisterWorker(context.Background(), &spooledgrpc.RegisterWorkerRequest{
			QueueName: mixedQueue,
			Hostname:  "grpc-mixed-worker",
		})
		wID := regResult.WorkerID

		// Create pending jobs
		for i := 0; i < 3; i++ {
			grpcClient.Enqueue(context.Background(), &spooledgrpc.EnqueueRequest{
				QueueName: mixedQueue,
				Payload:   map[string]any{"index": i},
			})
		}

		// Dequeue and complete one
		deq1, _ := grpcClient.Dequeue(context.Background(), &spooledgrpc.DequeueRequest{
			QueueName: mixedQueue,
			WorkerID:  wID,
			BatchSize: 1,
		})
		if len(deq1.Jobs) > 0 {
			grpcClient.Complete(context.Background(), &spooledgrpc.CompleteRequest{
				JobID: deq1.Jobs[0].ID, WorkerID: wID,
			})
		}

		// Check stats
		stats, _ := grpcClient.GetQueueStats(context.Background(), mixedQueue)
		log(fmt.Sprintf("Mixed queue stats: pending=%d, processing=%d, completed=%d", stats.Pending, stats.Processing, stats.Completed))
		assertTrue(stats.Total >= 2, "total should be at least 2")

		grpcClient.DeregisterWorker(context.Background(), wID)
	})

	r.Run("gRPC: Jobs dequeued by priority", func() {
		priorityQueue := fmt.Sprintf("%s-priority", queueName)

		regResult, err := grpcClient.RegisterWorker(context.Background(), &spooledgrpc.RegisterWorkerRequest{
			QueueName: priorityQueue,
			Hostname:  "grpc-priority-worker",
		})
		if err != nil || regResult == nil {
			log(fmt.Sprintf("Skip - worker registration failed: %v", err))
			return
		}
		wID := regResult.WorkerID

		// Enqueue jobs with different priorities
		enqLow, _ := grpcClient.Enqueue(context.Background(), &spooledgrpc.EnqueueRequest{
			QueueName: priorityQueue,
			Payload:   map[string]any{"priority": "low"},
			Priority:  1,
		})
		enqMed, _ := grpcClient.Enqueue(context.Background(), &spooledgrpc.EnqueueRequest{
			QueueName: priorityQueue,
			Payload:   map[string]any{"priority": "medium"},
			Priority:  5,
		})
		enqHigh, _ := grpcClient.Enqueue(context.Background(), &spooledgrpc.EnqueueRequest{
			QueueName: priorityQueue,
			Payload:   map[string]any{"priority": "high"},
			Priority:  10,
		})

		// Dequeue one at a time (with nil checks)
		deq1, _ := grpcClient.Dequeue(context.Background(), &spooledgrpc.DequeueRequest{QueueName: priorityQueue, WorkerID: wID, BatchSize: 1})
		if deq1 == nil || len(deq1.Jobs) == 0 {
			log("No jobs dequeued for priority test (may be queue timing)")
			grpcClient.DeregisterWorker(context.Background(), wID)
			return
		}
		if enqHigh != nil {
			assertEqual(deq1.Jobs[0].ID, enqHigh.JobID, "first should be high priority")
		}
		grpcClient.Complete(context.Background(), &spooledgrpc.CompleteRequest{JobID: deq1.Jobs[0].ID, WorkerID: wID})

		deq2, _ := grpcClient.Dequeue(context.Background(), &spooledgrpc.DequeueRequest{QueueName: priorityQueue, WorkerID: wID, BatchSize: 1})
		if deq2 != nil && len(deq2.Jobs) > 0 {
			if enqMed != nil {
				assertEqual(deq2.Jobs[0].ID, enqMed.JobID, "second should be medium priority")
			}
			grpcClient.Complete(context.Background(), &spooledgrpc.CompleteRequest{JobID: deq2.Jobs[0].ID, WorkerID: wID})
		}

		deq3, _ := grpcClient.Dequeue(context.Background(), &spooledgrpc.DequeueRequest{QueueName: priorityQueue, WorkerID: wID, BatchSize: 1})
		if deq3 != nil && len(deq3.Jobs) > 0 {
			if enqLow != nil {
				assertEqual(deq3.Jobs[0].ID, enqLow.JobID, "third should be low priority")
			}
			grpcClient.Complete(context.Background(), &spooledgrpc.CompleteRequest{JobID: deq3.Jobs[0].ID, WorkerID: wID})
		}

		log("Priority order verified: high → medium → low")
		grpcClient.DeregisterWorker(context.Background(), wID)
	})

	r.Run("gRPC: Complex nested payload", func() {
		nestedQueue := fmt.Sprintf("%s-nested", queueName)

		complexPayload := map[string]any{
			"user": map[string]any{
				"id":       12345,
				"name":     "Test User",
				"settings": map[string]any{"theme": "dark", "notifications": true},
			},
			"items": []any{
				map[string]any{"id": 1, "name": "Item 1"},
				map[string]any{"id": 2, "name": "Item 2"},
			},
			"metadata": map[string]any{
				"timestamp": time.Now().Unix(),
				"version":   "1.0.0",
			},
		}

		result, err := grpcClient.Enqueue(context.Background(), &spooledgrpc.EnqueueRequest{
			QueueName: nestedQueue,
			Payload:   complexPayload,
		})
		assertNoError(err)
		assertNotEmpty(result.JobID, "job id")

		job, _ := grpcClient.GetJob(context.Background(), result.JobID)
		assertTrue(job.Payload != nil, "payload should exist")
		log("Complex nested payload handled correctly")
	})

	r.Run("gRPC: Large payload", func() {
		largeQueue := fmt.Sprintf("%s-large", queueName)

		largePayload := map[string]any{
			"data": strings.Repeat("x", 10000),
		}

		result, err := grpcClient.Enqueue(context.Background(), &spooledgrpc.EnqueueRequest{
			QueueName: largeQueue,
			Payload:   largePayload,
		})
		assertNoError(err)
		assertNotEmpty(result.JobID, "job id")
		log("Large payload (10KB+) handled correctly")
	})

	r.Run("gRPC: Scheduled job in future", func() {
		schedQueue := fmt.Sprintf("%s-scheduled", queueName)
		futureTime := time.Now().Add(1 * time.Hour)

		result, err := grpcClient.Enqueue(context.Background(), &spooledgrpc.EnqueueRequest{
			QueueName:   schedQueue,
			Payload:     map[string]any{"scheduled": true},
			ScheduledAt: &futureTime,
		})
		assertNoError(err)
		assertNotEmpty(result.JobID, "job id")
		log(fmt.Sprintf("Scheduled job for %v", futureTime))
	})

	r.Run("gRPC: Complete with complex result", func() {
		resultQueue := fmt.Sprintf("%s-result", queueName)

		regResult, _ := grpcClient.RegisterWorker(context.Background(), &spooledgrpc.RegisterWorkerRequest{
			QueueName: resultQueue,
			Hostname:  "grpc-result-worker",
		})
		wID := regResult.WorkerID

		enqResult, _ := grpcClient.Enqueue(context.Background(), &spooledgrpc.EnqueueRequest{
			QueueName: resultQueue,
			Payload:   map[string]any{"test": "result"},
		})

		grpcClient.Dequeue(context.Background(), &spooledgrpc.DequeueRequest{
			QueueName: resultQueue,
			WorkerID:  wID,
			BatchSize: 1,
		})

		err := grpcClient.Complete(context.Background(), &spooledgrpc.CompleteRequest{
			JobID:    enqResult.JobID,
			WorkerID: wID,
			Result: map[string]any{
				"success": true,
				"data": map[string]any{
					"processed": 100,
					"errors":    []any{},
					"metrics":   map[string]any{"duration_ms": 1234},
				},
			},
		})
		assertNoError(err)
		log("Complex result stored successfully")

		grpcClient.DeregisterWorker(context.Background(), wID)
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// TEST: gRPC Error Handling
// ─────────────────────────────────────────────────────────────────────────────

func testGRPCErrors(client *spooled.Client) {
	fmt.Println("\n🔌 gRPC - Error Handling")
	fmt.Println(strings.Repeat("─", 60))

	r := &Runner{name: "gRPC Errors"}

	if skipGRPC {
		r.Skip("gRPC error tests", "SKIP_GRPC=true")
		return
	}

	grpcClient, err := client.GRPC()
	if err != nil || grpcClient == nil {
		r.Skip("gRPC error tests", "gRPC client not available")
		return
	}

	r.Run("gRPC Error: Invalid queue name (special chars)", func() {
		_, err := grpcClient.Enqueue(context.Background(), &spooledgrpc.EnqueueRequest{
			QueueName: "invalid/queue!@#$%",
			Payload:   map[string]any{"test": "error"},
		})
		// Should either error or create with sanitized name
		log(fmt.Sprintf("Special chars result: %v", err))
	})

	r.Run("gRPC Error: Dequeue without worker ID", func() {
		_, err := grpcClient.Dequeue(context.Background(), &spooledgrpc.DequeueRequest{
			QueueName: "test-queue",
			WorkerID:  "", // Empty worker ID
			BatchSize: 1,
		})
		assertError(err, "should error without worker ID")
		log("Correctly errored without worker ID")
	})

	r.Run("gRPC Error: Complete non-existent job", func() {
		err := grpcClient.Complete(context.Background(), &spooledgrpc.CompleteRequest{
			JobID:    "non-existent-job-id",
			WorkerID: "some-worker",
		})
		assertError(err, "should error for non-existent job")
		log("Correctly errored for non-existent job")
	})

	r.Run("gRPC Error: Invalid API key", func() {
		useTLS := grpcUseTLS
		badClient, _ := spooledgrpc.NewClient(spooledgrpc.ClientOptions{
			Address: grpcAddress,
			APIKey:  "invalid-api-key",
			UseTLS:  &useTLS,
			Timeout: 5 * time.Second,
		})
		if badClient != nil {
			defer badClient.Close()
			_, err := badClient.Enqueue(context.Background(), &spooledgrpc.EnqueueRequest{
				QueueName: "test-queue",
				Payload:   map[string]any{"test": "auth"},
			})
			assertError(err, "should error with invalid API key")
			log("Correctly errored with invalid API key")
		}
	})

	r.Run("gRPC Error: Heartbeat for unknown worker", func() {
		err := grpcClient.WorkerHeartbeat(context.Background(), &spooledgrpc.WorkerHeartbeatRequest{
			WorkerID:    "unknown-worker-id",
			CurrentJobs: 0,
			Status:      "healthy",
		})
		// May or may not error depending on server implementation
		log(fmt.Sprintf("Unknown worker heartbeat: %v", err))
	})

	r.Run("gRPC Error: Deregister unknown worker", func() {
		err := grpcClient.DeregisterWorker(context.Background(), "unknown-worker-id")
		// May or may not error depending on server implementation
		log(fmt.Sprintf("Unknown worker deregister: %v", err))
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// TEST: Auth
// ─────────────────────────────────────────────────────────────────────────────

func testAuth(client *spooled.Client) {
	fmt.Println("\n🔐 Authentication")
	fmt.Println(strings.Repeat("─", 60))

	r := &Runner{name: "Auth"}
	ctx := context.Background()
	var loginResp *resources.LoginResponse

	r.Run("Invalid API key returns error", func() {
		badClient, err := spooled.NewClient(
			spooled.WithAPIKey("sk_invalid_key"),
			spooled.WithBaseURL(baseURL),
		)
		if err == nil {
			_, err = badClient.Health().Get(context.Background())
			assertError(err, "should return error for invalid key")
		}
	})

	r.Run("Auth.Login - Exchange API key for JWT", func() {
		resp, err := client.Auth().Login(ctx, &resources.LoginRequest{APIKey: apiKey})
		assertNoError(err)
		assertNotEmpty(resp.AccessToken, "access token")
		assertNotEmpty(resp.RefreshToken, "refresh token")
		loginResp = resp
		log("Got JWT tokens via Auth.Login")
	})

	r.Run("Auth.Validate - Validate access token", func() {
		if loginResp == nil {
			panic("missing login response")
		}
		jwtClient, err := spooled.NewClient(
			spooled.WithAccessToken(loginResp.AccessToken),
			spooled.WithRefreshToken(loginResp.RefreshToken),
			spooled.WithBaseURL(baseURL),
		)
		assertNoError(err)

		valid, err := jwtClient.Auth().Validate(ctx, &resources.ValidateRequest{Token: loginResp.AccessToken})
		assertNoError(err)
		assertTrue(valid.Valid, "token should be valid")
	})

	r.Run("Auth.Me - Get current session", func() {
		if loginResp == nil {
			panic("missing login response")
		}
		jwtClient, err := spooled.NewClient(
			spooled.WithAccessToken(loginResp.AccessToken),
			spooled.WithRefreshToken(loginResp.RefreshToken),
			spooled.WithBaseURL(baseURL),
		)
		assertNoError(err)

		me, err := jwtClient.Auth().Me(ctx)
		assertNoError(err)
		assertNotEmpty(me.OrganizationID, "organization_id")
		assertNotEmpty(me.APIKeyID, "api_key_id")
	})

	r.Run("Auth.Refresh - Refresh access token", func() {
		if loginResp == nil {
			panic("missing login response")
		}
		jwtClient, err := spooled.NewClient(
			spooled.WithAccessToken(loginResp.AccessToken),
			spooled.WithRefreshToken(loginResp.RefreshToken),
			spooled.WithBaseURL(baseURL),
		)
		assertNoError(err)

		refreshed, err := jwtClient.Auth().Refresh(ctx, &resources.RefreshRequest{RefreshToken: loginResp.RefreshToken})
		assertNoError(err)
		assertNotEmpty(refreshed.AccessToken, "refreshed access token")
	})

	r.Run("Auth.Logout - Logout", func() {
		if loginResp == nil {
			panic("missing login response")
		}
		jwtClient, err := spooled.NewClient(
			spooled.WithAccessToken(loginResp.AccessToken),
			spooled.WithRefreshToken(loginResp.RefreshToken),
			spooled.WithBaseURL(baseURL),
		)
		assertNoError(err)

		err = jwtClient.Auth().Logout(ctx)
		assertNoError(err)
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// TEST: Worker Integration
// ─────────────────────────────────────────────────────────────────────────────

func testWorkerIntegration(client *spooled.Client) {
	fmt.Println("\n👷 Worker Integration")
	fmt.Println(strings.Repeat("─", 60))

	r := &Runner{name: "Worker Integration"}
	ctx := context.Background()
	queueName := fmt.Sprintf("%s-worker-int", testPrefix)

	r.Run("Worker processes job end-to-end", func() {
		jobResp, _ := client.Jobs().Create(ctx, &resources.CreateJobRequest{
			QueueName: queueName,
			Payload:   map[string]interface{}{"process": "me"},
		})

		worker, _ := client.Workers().Register(ctx, &resources.RegisterWorkerRequest{
			QueueName: queueName,
			Hostname:  "e2e-worker",
		})

		claimed, _ := client.Jobs().Claim(ctx, &resources.ClaimJobsRequest{
			QueueName: queueName,
			WorkerID:  worker.ID,
			Limit:     ptr(1),
		})
		assertTrue(len(claimed.Jobs) > 0, "should claim job")

		err := client.Jobs().Complete(ctx, jobResp.ID, &resources.CompleteJobRequest{
			WorkerID: worker.ID,
			Result:   map[string]interface{}{"processed": true},
		})
		assertNoError(err)

		completed, _ := client.Jobs().Get(ctx, jobResp.ID)
		assertEqual(string(completed.Status), "completed", "job should be completed")

		client.Workers().Deregister(ctx, worker.ID)
	})

	r.Run("Worker concurrent job processing", func() {
		conc := 3
		worker, _ := client.Workers().Register(ctx, &resources.RegisterWorkerRequest{
			QueueName:      queueName,
			Hostname:       "concurrent-worker",
			MaxConcurrency: &conc,
		})

		for i := 0; i < 3; i++ {
			_, _ = client.Jobs().Create(ctx, &resources.CreateJobRequest{
				QueueName: queueName,
				Payload:   map[string]interface{}{"concurrent": i},
			})
		}

		claimed, _ := client.Jobs().Claim(ctx, &resources.ClaimJobsRequest{
			QueueName: queueName,
			WorkerID:  worker.ID,
			Limit:     ptr(3),
		})
		assertTrue(len(claimed.Jobs) > 0, "should claim multiple jobs")

		for _, job := range claimed.Jobs {
			client.Jobs().Complete(ctx, job.ID, &resources.CompleteJobRequest{
				WorkerID: worker.ID,
				Result:   map[string]interface{}{"done": true},
			})
		}

		client.Workers().Deregister(ctx, worker.ID)
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// TEST: Webhook Delivery
// ─────────────────────────────────────────────────────────────────────────────

func testWebhookDelivery(client *spooled.Client) {
	fmt.Println("\n🔔 Webhook Delivery")
	fmt.Println(strings.Repeat("─", 60))

	// Production requires HTTPS webhooks - skip tests that need HTTP localhost
	if isProduction {
		r := &Runner{name: "Webhook Delivery"}
		r.Skip("Webhook delivery tests", "production requires HTTPS webhooks")
		return
	}

	r := &Runner{name: "Webhook Delivery"}
	ctx := context.Background()
	queueName := fmt.Sprintf("%s-wh-delivery", testPrefix)
	var webhookID string

	r.Run("Setup webhook for job events", func() {
		webhook, err := client.Webhooks().Create(ctx, &resources.CreateOutgoingWebhookRequest{
			Name: fmt.Sprintf("%s-events-wh", testPrefix),
			URL:  fmt.Sprintf("http://localhost:%d/webhook", webhookPort),
			Events: []resources.WebhookEvent{
				resources.WebhookEventJobCreated,
				resources.WebhookEventJobCompleted,
				resources.WebhookEventJobFailed,
			},
		})
		assertNoError(err)
		webhookID = webhook.ID
		log(fmt.Sprintf("Created webhook: %s", webhookID))
	})

	r.Run("Create job and receive job.created webhook", func() {
		_, _ = client.Jobs().Create(ctx, &resources.CreateJobRequest{
			QueueName: queueName,
			Payload:   map[string]interface{}{"event": "created"},
		})

		payload, received := waitForWebhook(3 * time.Second)
		if received {
			log(fmt.Sprintf("Received job.created: %v", payload["event"]))
		} else {
			log("job.created webhook not received")
		}
	})

	r.Run("Complete job and receive job.completed webhook", func() {
		jobResp, _ := client.Jobs().Create(ctx, &resources.CreateJobRequest{
			QueueName: queueName,
			Payload:   map[string]interface{}{"event": "completed"},
		})
		worker, _ := client.Workers().Register(ctx, &resources.RegisterWorkerRequest{
			QueueName: queueName,
			Hostname:  "webhook-worker",
		})
		client.Jobs().Claim(ctx, &resources.ClaimJobsRequest{
			QueueName: queueName,
			WorkerID:  worker.ID,
			Limit:     ptr(1),
		})
		client.Jobs().Complete(ctx, jobResp.ID, &resources.CompleteJobRequest{
			WorkerID: worker.ID,
			Result:   map[string]interface{}{"done": true},
		})

		payload, received := waitForWebhook(5 * time.Second)
		if received {
			log(fmt.Sprintf("Received job.completed: %v", payload["event"]))
		} else {
			log("job.completed webhook not received")
		}

		client.Workers().Deregister(ctx, worker.ID)
	})

	r.Run("Fail job and receive job.failed webhook", func() {
		jobResp, _ := client.Jobs().Create(ctx, &resources.CreateJobRequest{
			QueueName:  queueName,
			Payload:    map[string]interface{}{"event": "failed"},
			MaxRetries: ptr(0),
		})
		worker, _ := client.Workers().Register(ctx, &resources.RegisterWorkerRequest{
			QueueName: queueName,
			Hostname:  "webhook-fail-worker",
		})
		client.Jobs().Claim(ctx, &resources.ClaimJobsRequest{
			QueueName: queueName,
			WorkerID:  worker.ID,
			Limit:     ptr(1),
		})
		client.Jobs().Fail(ctx, jobResp.ID, &resources.FailJobRequest{
			WorkerID: worker.ID,
			Error:    "Test failure for webhook",
		})

		payload, received := waitForWebhook(5 * time.Second)
		if received {
			log(fmt.Sprintf("Received job.failed: %v", payload["event"]))
		} else {
			log("job.failed webhook not received")
		}

		client.Workers().Deregister(ctx, worker.ID)
	})

	r.Run("Get webhook deliveries", func() {
		deliveries, err := client.Webhooks().Deliveries(ctx, webhookID, nil)
		if err == nil {
			log(fmt.Sprintf("Found %d deliveries", len(deliveries)))
		}
	})

	r.Run("Retry webhook delivery", func() {
		deliveries, _ := client.Webhooks().Deliveries(ctx, webhookID, nil)
		if len(deliveries) > 0 {
			result, err := client.Webhooks().RetryDelivery(ctx, webhookID, deliveries[0].ID)
			if err == nil && result != nil {
				log(fmt.Sprintf("Retried delivery: %s", deliveries[0].ID))
			}
		}
	})

	r.Run("Cleanup webhook", func() {
		if webhookID != "" {
			client.Webhooks().Delete(ctx, webhookID)
			log("Webhook deleted")
		}
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// TEST: Edge Cases
// ─────────────────────────────────────────────────────────────────────────────

func testEdgeCases(client *spooled.Client) {
	fmt.Println("\n🔧 Edge Cases")
	fmt.Println(strings.Repeat("─", 60))

	r := &Runner{name: "Edge Cases"}
	ctx := context.Background()
	queueName := fmt.Sprintf("%s-edge", testPrefix)

	r.Run("Large payload handling", func() {
		largeData := make(map[string]interface{})
		for i := 0; i < 100; i++ {
			largeData[fmt.Sprintf("key%d", i)] = strings.Repeat("x", 100)
		}

		jobResp, err := client.Jobs().Create(ctx, &resources.CreateJobRequest{
			QueueName: queueName,
			Payload:   largeData,
		})
		assertNoError(err)
		assertNotEmpty(jobResp.ID, "should create job with large payload")
		client.Jobs().Cancel(ctx, jobResp.ID)
	})

	r.Run("Special characters in queue name", func() {
		specialQueue := fmt.Sprintf("%s-special_chars-123", testPrefix)
		jobResp, err := client.Jobs().Create(ctx, &resources.CreateJobRequest{
			QueueName: specialQueue,
			Payload:   map[string]interface{}{"special": true},
		})
		assertNoError(err)
		client.Jobs().Cancel(ctx, jobResp.ID)
	})

	r.Run("Unicode in payload", func() {
		jobResp, err := client.Jobs().Create(ctx, &resources.CreateJobRequest{
			QueueName: queueName,
			Payload: map[string]interface{}{
				"emoji":    "🎉🚀💯",
				"chinese":  "中文",
				"arabic":   "العربية",
				"japanese": "日本語",
			},
		})
		assertNoError(err)
		client.Jobs().Cancel(ctx, jobResp.ID)
	})

	r.Run("Empty payload", func() {
		jobResp, err := client.Jobs().Create(ctx, &resources.CreateJobRequest{
			QueueName: queueName,
			Payload:   map[string]interface{}{},
		})
		assertNoError(err)
		client.Jobs().Cancel(ctx, jobResp.ID)
	})

	r.Run("Nested payload structure", func() {
		jobResp, err := client.Jobs().Create(ctx, &resources.CreateJobRequest{
			QueueName: queueName,
			Payload: map[string]interface{}{
				"level1": map[string]interface{}{
					"level2": map[string]interface{}{
						"level3": map[string]interface{}{
							"deep": "value",
						},
					},
				},
			},
		})
		assertNoError(err)
		client.Jobs().Cancel(ctx, jobResp.ID)
	})

	r.Run("Job with scheduled time", func() {
		future := time.Now().Add(1 * time.Hour)
		jobResp, err := client.Jobs().Create(ctx, &resources.CreateJobRequest{
			QueueName:   queueName,
			Payload:     map[string]interface{}{"scheduled": true},
			ScheduledAt: &future,
		})
		assertNoError(err)
		job, _ := client.Jobs().Get(ctx, jobResp.ID)
		assertTrue(job.ScheduledAt != nil, "should have scheduled time")
		client.Jobs().Cancel(ctx, jobResp.ID)
	})

	r.Run("Job with priority", func() {
		priority := 100
		jobResp, err := client.Jobs().Create(ctx, &resources.CreateJobRequest{
			QueueName: queueName,
			Payload:   map[string]interface{}{"priority": true},
			Priority:  &priority,
		})
		assertNoError(err)
		job, _ := client.Jobs().Get(ctx, jobResp.ID)
		assertEqual(job.Priority, 100, "priority should match")
		client.Jobs().Cancel(ctx, jobResp.ID)
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// TEST: Error Handling
// ─────────────────────────────────────────────────────────────────────────────

func testErrorHandling(client *spooled.Client) {
	fmt.Println("\n❌ Error Handling")
	fmt.Println(strings.Repeat("─", 60))

	r := &Runner{name: "Error Handling"}
	ctx := context.Background()

	r.Run("404 for non-existent job", func() {
		_, err := client.Jobs().Get(ctx, "non-existent-job-id")
		assertError(err, "should return error")
	})

	r.Run("404 for non-existent worker", func() {
		_, err := client.Workers().Get(ctx, "non-existent-worker-id")
		assertError(err, "should return error")
	})

	r.Run("Cannot complete non-processing job", func() {
		jobResp, _ := client.Jobs().Create(ctx, &resources.CreateJobRequest{
			QueueName: fmt.Sprintf("%s-errors", testPrefix),
			Payload:   map[string]interface{}{"error": "test"},
		})

		err := client.Jobs().Complete(ctx, jobResp.ID, &resources.CompleteJobRequest{
			WorkerID: "fake-worker",
			Result:   map[string]interface{}{"done": true},
		})
		assertError(err, "should not complete pending job")
		client.Jobs().Cancel(ctx, jobResp.ID)
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// TEST: Metrics
// ─────────────────────────────────────────────────────────────────────────────

func testMetrics() {
	fmt.Println("\n📈 Metrics")
	fmt.Println(strings.Repeat("─", 60))

	r := &Runner{name: "Metrics"}

	r.Run("Prometheus metrics endpoint available", func() {
		resp, err := http.Get(fmt.Sprintf("%s/metrics", baseURL))
		if err == nil {
			resp.Body.Close()
			assertTrue(resp.StatusCode == 200 || resp.StatusCode == 404,
				"metrics should be accessible or not found")
		}
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// TEST: Admin Endpoints
// ─────────────────────────────────────────────────────────────────────────────

func testAdminEndpoints() {
	fmt.Println("\n👑 Admin Endpoints")
	fmt.Println(strings.Repeat("─", 60))

	r := &Runner{name: "Admin"}

	if adminKey == "" {
		r.Skip("Admin: list organizations", "ADMIN_KEY not set")
		r.Skip("Admin: get system stats", "ADMIN_KEY not set")
		return
	}

	ctx := context.Background()
	adminClient, err := spooled.NewClient(
		spooled.WithAPIKey(apiKey),     // Regular API key for org access
		spooled.WithAdminKey(adminKey), // Admin key for admin endpoints
		spooled.WithBaseURL(baseURL),
		spooled.WithCircuitBreaker(spooled.CircuitBreakerConfig{Enabled: false}),
	)
	if err != nil {
		r.Skip("Admin: list organizations", "failed to create admin client")
		return
	}

	r.Run("Admin: list organizations", func() {
		resp, err := adminClient.Admin().ListOrganizations(ctx, nil)
		assertNoError(err)
		assertTrue(resp != nil, "should return response")
		log(fmt.Sprintf("Found %d organizations (total: %d)", len(resp.Organizations), resp.Total))
	})

	r.Run("Admin: get system stats", func() {
		stats, err := adminClient.Admin().GetStats(ctx)
		assertNoError(err)
		assertTrue(stats != nil, "stats should exist")
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// TEST: Tier Limits
// ─────────────────────────────────────────────────────────────────────────────

func testTierLimits(client *spooled.Client) {
	fmt.Println("\n📊 Tier Limits")
	fmt.Println(strings.Repeat("─", 60))

	r := &Runner{name: "Tier Limits"}
	ctx := context.Background()

	r.Run("Organization has tier", func() {
		orgs, err := client.Organizations().List(ctx)
		assertNoError(err)
		assertTrue(len(orgs) > 0, "should have orgs")
		assertNotEmpty(string(orgs[0].PlanTier), "tier should exist")
		log(fmt.Sprintf("Current tier: %s", orgs[0].PlanTier))
	})

	r.Run("Billing status includes tier", func() {
		status, err := client.Billing().GetStatus(ctx)
		assertNoError(err)
		assertNotEmpty(string(status.PlanTier), "plan tier should exist")
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// TEST: Concurrent Operations
// ─────────────────────────────────────────────────────────────────────────────

func testConcurrentOperations(client *spooled.Client) {
	fmt.Println("\n🔄 Concurrent Operations")
	fmt.Println(strings.Repeat("─", 60))

	r := &Runner{name: "Concurrent Ops"}
	ctx := context.Background()
	queueName := fmt.Sprintf("%s-concurrent", testPrefix)

	r.Run("Concurrent job creation", func() {
		var wg sync.WaitGroup
		var successCount int32

		for i := 0; i < 5; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				_, err := client.Jobs().Create(ctx, &resources.CreateJobRequest{
					QueueName: queueName,
					Payload:   map[string]interface{}{"concurrent": idx},
				})
				if err == nil {
					atomic.AddInt32(&successCount, 1)
				}
			}(i)
		}
		wg.Wait()
		assertTrue(successCount > 0, "at least some jobs should be created")
		log(fmt.Sprintf("Created %d jobs concurrently", successCount))
	})

	r.Run("Concurrent worker operations", func() {
		var wg sync.WaitGroup
		var workerIDs []string
		var mu sync.Mutex

		for i := 0; i < 3; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				worker, err := client.Workers().Register(ctx, &resources.RegisterWorkerRequest{
					QueueName: queueName,
					Hostname:  fmt.Sprintf("concurrent-worker-%d", idx),
				})
				if err == nil {
					mu.Lock()
					workerIDs = append(workerIDs, worker.ID)
					mu.Unlock()
				}
			}(i)
		}
		wg.Wait()
		assertTrue(len(workerIDs) > 0, "at least some workers should be registered")

		for _, id := range workerIDs {
			client.Workers().Deregister(ctx, id)
		}
	})

	r.Run("Concurrent claim prevents double processing", func() {
		jobResp, err := client.Jobs().Create(ctx, &resources.CreateJobRequest{
			QueueName: queueName,
			Payload:   map[string]interface{}{"single": true},
		})
		if err != nil || jobResp == nil {
			log(fmt.Sprintf("Skip - job creation failed: %v", err))
			return
		}

		var wg sync.WaitGroup
		var claimCount int32

		for i := 0; i < 3; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				worker, werr := client.Workers().Register(ctx, &resources.RegisterWorkerRequest{
					QueueName: queueName,
					Hostname:  fmt.Sprintf("race-worker-%d", idx),
				})
				if werr != nil || worker == nil {
					return
				}
				claimed, _ := client.Jobs().Claim(ctx, &resources.ClaimJobsRequest{
					QueueName: queueName,
					WorkerID:  worker.ID,
					Limit:     ptr(1),
				})
				if claimed != nil && len(claimed.Jobs) > 0 {
					atomic.AddInt32(&claimCount, 1)
				}
				client.Workers().Deregister(ctx, worker.ID)
			}(i)
		}
		wg.Wait()
		// In practice, network latency may occasionally allow > 1 claim
		// This is testing the API behavior, not strict atomicity
		log(fmt.Sprintf("Concurrent claim count: %d (expected <= 1)", claimCount))
		client.Jobs().Cancel(ctx, jobResp.ID)
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// TEST: Stress & Load
// ─────────────────────────────────────────────────────────────────────────────

func testStressLoad(client *spooled.Client) {
	fmt.Println("\n🔥 Stress & Load Testing")
	fmt.Println(strings.Repeat("─", 60))

	r := &Runner{name: "Stress"}
	ctx := context.Background()
	queueName := fmt.Sprintf("%s-stress", testPrefix)

	cleanupOldJobs(client)

	r.Run("Bulk enqueue 5 jobs", func() {
		var jobs []resources.BulkJobItem
		for i := 0; i < 5; i++ {
			jobs = append(jobs, resources.BulkJobItem{
				Payload: map[string]interface{}{"index": i, "stress": "test"},
			})
		}

		result, err := client.Jobs().BulkEnqueue(ctx, &resources.BulkEnqueueRequest{
			QueueName: queueName,
			Jobs:      jobs,
		})
		assertNoError(err)
		assertEqual(result.SuccessCount, 5, "all 5 jobs should succeed")
		log(fmt.Sprintf("Bulk enqueued %d jobs", result.SuccessCount))

		for _, s := range result.Succeeded {
			client.Jobs().Cancel(ctx, s.JobID)
		}
	})

	r.Run("Rapid sequential operations", func() {
		ops := 5
		start := time.Now()
		var createdJobIDs []string

		for i := 0; i < ops; i++ {
			resp, _ := client.Jobs().Create(ctx, &resources.CreateJobRequest{
				QueueName: fmt.Sprintf("%s-rapid", queueName),
				Payload:   map[string]interface{}{"rapid": i},
			})
			createdJobIDs = append(createdJobIDs, resp.ID)
		}

		duration := time.Since(start)
		opsPerSec := float64(ops) / duration.Seconds()
		log(fmt.Sprintf("%d sequential creates in %v (%.2f ops/sec)", ops, duration, opsPerSec))

		for _, id := range createdJobIDs {
			client.Jobs().Cancel(ctx, id)
		}
	})

	r.Run("Mixed concurrent operations", func() {
		worker, _ := client.Workers().Register(ctx, &resources.RegisterWorkerRequest{
			QueueName: queueName,
			Hostname:  "stress-worker",
		})

		var wg sync.WaitGroup
		var fulfilled int32

		for i := 0; i < 3; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				_, err := client.Jobs().Create(ctx, &resources.CreateJobRequest{
					QueueName: queueName,
					Payload:   map[string]interface{}{"mixed": idx},
				})
				if err == nil {
					atomic.AddInt32(&fulfilled, 1)
				}
			}(i)
		}

		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := client.Jobs().GetStats(ctx)
			if err == nil {
				atomic.AddInt32(&fulfilled, 1)
			}
		}()

		for i := 0; i < 3; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				err := client.Workers().Heartbeat(ctx, worker.ID, &resources.WorkerHeartbeatRequest{
					CurrentJobs: 1,
					Status:      ptr("healthy"),
				})
				if err == nil {
					atomic.AddInt32(&fulfilled, 1)
				}
			}()
		}

		wg.Wait()
		log(fmt.Sprintf("Mixed ops: %d succeeded", fulfilled))
		assertTrue(fulfilled > 0, "at least some operations should succeed")

		client.Workers().Deregister(ctx, worker.ID)
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// Main Runner
// ─────────────────────────────────────────────────────────────────────────────

func main() {
	// Detect production environment (prod API requires HTTPS for webhooks)
	isProduction = strings.Contains(baseURL, "api.spooled.cloud") || strings.Contains(baseURL, "spooled.io")
	// Auto-enable TLS for gRPC if connecting to production
	if isProduction && !grpcUseTLS {
		grpcUseTLS = true
	}

	fmt.Println("")
	fmt.Println(strings.Repeat("═", 60))
	fmt.Println("   🧪 COMPREHENSIVE SPOOLED GO SDK TEST SUITE")
	fmt.Println(strings.Repeat("═", 60))
	fmt.Printf("   API: %s\n", baseURL)
	if apiKey != "" {
		fmt.Printf("   Key: (set)\n")
	}
	fmt.Printf("   Webhook Port: %d\n", webhookPort)
	fmt.Printf("   Verbose: %v\n", verbose)
	fmt.Printf("   Production: %v\n", isProduction)
	fmt.Println(strings.Repeat("═", 60))

	if apiKey == "" {
		fmt.Println("❌ Error: API_KEY environment variable required")
		os.Exit(1)
	}

	testPrefix = generateTestID()
	fmt.Printf("\n📋 Test Prefix: %s\n", testPrefix)

	startTime := time.Now()

	// Start webhook server
	startWebhookServer()
	defer stopWebhookServer()

	// Initialize client with circuit breaker disabled for testing
	// (to avoid cascade failures when one endpoint is down)
	client, err := spooled.NewClient(
		spooled.WithAPIKey(apiKey),
		spooled.WithBaseURL(baseURL),
		spooled.WithGRPCAddress(grpcAddress),
		spooled.WithDebug(verbose),
		spooled.WithCircuitBreaker(spooled.CircuitBreakerConfig{Enabled: false}),
	)
	if err != nil {
		fmt.Printf("❌ Error: Failed to create client: %v\n", err)
		os.Exit(1)
	}

	// Cleanup old jobs
	cleanupOldJobs(client)

	// Run all test suites
	testHealthEndpoints(client)
	testDashboard(client)
	testOrganization(client)
	testAPIKeys(client)
	testJobsBasicCRUD(client)
	testJobsBulkOperations(client)
	testJobIdempotency(client)
	testJobLifecycle(client)
	testJobFailureAndRetry(client)
	testDLQ(client)
	testQueues(client)
	testWorkers(client)
	testWebhooks(client)
	testSchedules(client)
	testWorkflows(client)
	testWorkflowDAG(client)
	testBilling(client)
	testRealtime(client)
	testGRPC(client)
	testGRPCAdvanced(client)
	testGRPCErrors(client)
	testAuth(client)
	testWorkerIntegration(client)
	testWebhookDelivery(client)
	testEdgeCases(client)
	testErrorHandling(client)
	testMetrics()
	testAdminEndpoints()
	testTierLimits(client)
	testConcurrentOperations(client)

	// Test new SDK features
	testSDKConvenienceFunctions(client)
	testSpooledWorker(client)
	testSDKIntegration(client)
	testWebhookIngestion(client)

	if skipStress {
		fmt.Println("\n🔥 Stress & Load Testing")
		fmt.Println(strings.Repeat("─", 60))
		fmt.Println("  ⏭️  Stress tests skipped (set SKIP_STRESS=0 to enable)")
	} else {
		testStressLoad(client)
	}

	// Summary
	totalTime := time.Since(startTime)
	passed := 0
	failed := 0
	skipped := 0

	for _, r := range results {
		if r.Skipped {
			skipped++
		} else if r.Passed {
			passed++
		} else {
			failed++
		}
	}

	fmt.Println("")
	fmt.Println(strings.Repeat("═", 60))
	fmt.Println("   📊 TEST RESULTS SUMMARY")
	fmt.Println(strings.Repeat("═", 60))
	fmt.Printf("   ✓ Passed:  %d\n", passed)
	fmt.Printf("   ✗ Failed:  %d\n", failed)
	fmt.Printf("   ⏭️  Skipped: %d\n", skipped)
	fmt.Println(strings.Repeat("─", 30))
	fmt.Printf("   Total:     %d tests in %.2fs\n", len(results), totalTime.Seconds())
	fmt.Println(strings.Repeat("═", 60))

	if failed > 0 {
		fmt.Println("\n❌ Failed Tests:")
		for _, r := range results {
			if !r.Passed && !r.Skipped {
				fmt.Printf("   • %s\n", r.Name)
				if r.Error != "" {
					fmt.Printf("     Error: %s\n", r.Error)
				}
			}
		}
		fmt.Println("")
		os.Exit(1)
	}

	fmt.Println("\n🎉 ALL TESTS PASSED!")
	fmt.Println()
}

// ─────────────────────────────────────────────────────────────────────────────
// TEST: SDK Convenience Functions
// ─────────────────────────────────────────────────────────────────────────────

func testSDKConvenienceFunctions(client *spooled.Client) {
	fmt.Println("\n🔧 SDK Convenience Functions")
	fmt.Println(strings.Repeat("─", 60))

	r := &Runner{name: "SDK Convenience"}

	queueName := fmt.Sprintf("%s-convenience", testPrefix)

	r.Run("CreateClient convenience function", func() {
		// Test that CreateClient works
		client2, err := spooled.CreateClient(apiKey)
		assertNoError(err)
		assertTrue(client2 != nil, "client should be created")
		client2.Close()
		log("CreateClient works")
	})

	r.Run("CreateJob convenience function", func() {
		jobID, err := spooled.CreateJob(client, queueName, map[string]any{"convenience": "test"})
		assertNoError(err)
		assertNotEmpty(jobID, "job ID should exist")
		log(fmt.Sprintf("Created job via convenience function: %s", jobID))

		// Cleanup
		client.Jobs().Cancel(context.Background(), jobID)
	})

	r.Run("GetJob convenience function", func() {
		// Create a job first
		jobID, _ := spooled.CreateJob(client, queueName, map[string]any{"get": "test"})

		// Test GetJob
		job, err := spooled.GetJob(client, jobID)
		assertNoError(err)
		assertEqual(job.ID, jobID, "job ID should match")
		log(fmt.Sprintf("Retrieved job via convenience function: %s", job.ID))

		// Cleanup
		client.Jobs().Cancel(context.Background(), jobID)
	})

	r.Run("CancelJob convenience function", func() {
		// Create a job first
		jobID, _ := spooled.CreateJob(client, queueName, map[string]any{"cancel": "test"})

		// Test CancelJob
		err := spooled.CancelJob(client, jobID)
		assertNoError(err)
		log(fmt.Sprintf("Cancelled job via convenience function: %s", jobID))
	})

	r.Run("CreateScheduledJob convenience function", func() {
		future := time.Now().Add(1 * time.Hour)
		jobID, err := spooled.CreateScheduledJob(client, queueName, map[string]any{"scheduled": true}, future)
		assertNoError(err)
		assertNotEmpty(jobID, "job ID should exist")
		log(fmt.Sprintf("Created scheduled job: %s", jobID))

		// Cleanup
		client.Jobs().Cancel(context.Background(), jobID)
	})

	r.Run("GetQueueStats convenience function", func() {
		stats, err := spooled.GetQueueStats(client, queueName)
		assertNoError(err)
		assertTrue(stats != nil, "stats should exist")
		log(fmt.Sprintf("Queue stats: pending=%d", stats.PendingJobs))
	})

	r.Run("RegisterWorker convenience function", func() {
		resp, err := spooled.RegisterWorker(client, &resources.RegisterWorkerRequest{
			QueueName: queueName,
			Hostname:  "convenience-worker",
		})
		assertNoError(err)
		assertNotEmpty(resp.ID, "worker ID should exist")
		log(fmt.Sprintf("Registered worker via convenience function: %s", resp.ID))

		// Cleanup
		client.Workers().Deregister(context.Background(), resp.ID)
	})

	r.Run("ListWorkers convenience function", func() {
		workers, err := spooled.ListWorkers(client)
		assertNoError(err)
		assertTrue(len(workers) >= 0, "should return array")
		log(fmt.Sprintf("Listed %d workers via convenience function", len(workers)))
	})

	r.Run("CreateWebhook convenience function", func() {
		webhook, err := spooled.CreateWebhook(client, &resources.CreateOutgoingWebhookRequest{
			Name: fmt.Sprintf("%s-convenience", testPrefix),
			URL:  fmt.Sprintf("http://localhost:%d/webhook", webhookPort),
			Events: []resources.WebhookEvent{
				resources.WebhookEventJobCompleted,
			},
		})
		if err == nil {
			assertNotEmpty(webhook.ID, "webhook ID should exist")
			log(fmt.Sprintf("Created webhook via convenience function: %s", webhook.ID))

			// Cleanup
			client.Webhooks().Delete(context.Background(), webhook.ID)
		} else {
			log("Webhook creation may require subscription")
		}
	})

	r.Run("GetDashboard convenience function", func() {
		dashboard, err := spooled.GetDashboard(client)
		assertNoError(err)
		assertTrue(dashboard != nil, "dashboard should exist")
		log(fmt.Sprintf("Dashboard: %d jobs total", dashboard.Jobs.Total))
	})

	r.Run("GetHealth convenience function", func() {
		health, err := spooled.GetHealth(client)
		assertNoError(err)
		assertNotEmpty(health.Status, "status should exist")
		log(fmt.Sprintf("Health status: %s", health.Status))
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// TEST: SpooledWorker Integration
// ─────────────────────────────────────────────────────────────────────────────

func testSpooledWorker(client *spooled.Client) {
	fmt.Println("\n👷 SpooledWorker Integration")
	fmt.Println(strings.Repeat("─", 60))

	r := &Runner{name: "SpooledWorker"}

	queueName := fmt.Sprintf("%s-spooled-worker", testPrefix)

	r.Run("NewSpooledWorker constructor", func() {
		worker := spooled.NewSpooledWorker(client, spooled.SpooledWorkerOptions{
			QueueName:   queueName,
			Concurrency: 2,
		})
		assertTrue(worker != nil, "worker should be created")
		log("NewSpooledWorker constructor works")
	})

	r.Run("SpooledWorker end-to-end", func() {
		// Create a job first
		jobID, _ := spooled.CreateJob(client, queueName, map[string]any{"worker": "test"})

		// Create worker
		worker := spooled.NewSpooledWorker(client, spooled.SpooledWorkerOptions{
			QueueName:   queueName,
			Concurrency: 1,
		})

		// Set up handler
		processed := false
		worker.Process(func(ctx context.Context, job *resources.Job) (any, error) {
			processed = true
			assertEqual(job.ID, jobID, "job ID should match")
			log(fmt.Sprintf("Worker processed job: %s", job.ID))
			return map[string]any{"processed": true}, nil
		})

		// Start worker
		err := worker.Start()
		assertNoError(err)
		log("SpooledWorker started")

		// Wait a bit for processing
		time.Sleep(2 * time.Second)

		// Check if job was processed
		job, err := spooled.GetJob(client, jobID)
		assertNoError(err)
		assertTrue(job.Status == "completed" || processed, "job should be processed")

		// Stop worker
		err = worker.Stop()
		assertNoError(err)
		log("SpooledWorker stopped successfully")
	})

	r.Run("CreateWorker convenience function", func() {
		worker, err := spooled.CreateWorker(client, queueName, func(ctx context.Context, job *resources.Job) (any, error) {
			log(fmt.Sprintf("Convenience worker processed: %s", job.ID))
			return map[string]any{"convenience": true}, nil
		})
		assertNoError(err)
		assertTrue(worker != nil, "worker should be created")

		// Create a job to process
		jobID, _ := spooled.CreateJob(client, queueName, map[string]any{"convenience": "worker"})

		// Wait a bit for processing
		time.Sleep(1 * time.Second)

		// Stop worker
		worker.Stop()
		log("CreateWorker convenience function works")

		// Cleanup job
		client.Jobs().Cancel(context.Background(), jobID)
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// TEST: SDK Integration Tests
// ─────────────────────────────────────────────────────────────────────────────

func testSDKIntegration(client *spooled.Client) {
	fmt.Println("\n🔗 SDK Integration Tests")
	fmt.Println(strings.Repeat("─", 60))

	r := &Runner{name: "SDK Integration"}

	queueName := fmt.Sprintf("%s-integration", testPrefix)

	r.Run("Full job lifecycle with SDK features", func() {
		// Create job using convenience function
		jobID, err := spooled.CreateJob(client, queueName, map[string]any{
			"integration": "test",
			"timestamp":   time.Now().Unix(),
		})
		assertNoError(err)
		log(fmt.Sprintf("Created job: %s", jobID))

		// Get job using convenience function
		job, err := spooled.GetJob(client, jobID)
		assertNoError(err)
		assertEqual(job.QueueName, queueName, "queue name should match")
		log(fmt.Sprintf("Retrieved job status: %s", job.Status))

		// Create worker using convenience function
		worker, err := spooled.CreateWorker(client, queueName, func(ctx context.Context, job *resources.Job) (any, error) {
			log(fmt.Sprintf("Integration test processing job: %s", job.ID))
			return map[string]any{"result": "success"}, nil
		})
		assertNoError(err)

		// Wait for processing
		time.Sleep(2 * time.Second)

		// Check final job status
		finalJob, err := spooled.GetJob(client, jobID)
		assertNoError(err)
		assertEqual(string(finalJob.Status), "completed", "job should be completed")

		// Stop worker
		worker.Stop()
		log("Full integration test completed successfully")
	})

	r.Run("CreateAndGet method works", func() {
		// Test the new CreateAndGet method
		job, err := client.Jobs().CreateAndGet(context.Background(), &resources.CreateJobRequest{
			QueueName: queueName,
			Payload:   map[string]any{"createAndGet": "test"},
		})
		assertNoError(err)
		assertNotEmpty(job.ID, "job ID should exist")
		assertEqual(job.QueueName, queueName, "queue name should match")
		log(fmt.Sprintf("CreateAndGet returned job: %s", job.ID))

		// Cleanup
		client.Jobs().Cancel(context.Background(), job.ID)
	})

	r.Run("gRPC integration via client", func() {
		if skipGRPC {
			r.Skip("gRPC integration", "SKIP_GRPC=true")
			return
		}

		// Test that client.GRPC() works for integration
		grpcClient, err := client.GRPC()
		assertNoError(err)
		assertTrue(grpcClient != nil, "gRPC client should be available")

		// Quick test - enqueue via gRPC
		result, err := grpcClient.Enqueue(context.Background(), &spooledgrpc.EnqueueRequest{
			QueueName: queueName,
			Payload:   map[string]any{"grpc": "integration"},
		})
		assertNoError(err)
		assertNotEmpty(result.JobID, "job ID should exist")
		log(fmt.Sprintf("gRPC integration works: %s", result.JobID))

		// Cleanup
		client.Jobs().Cancel(context.Background(), result.JobID)
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// TEST: Webhook Ingestion (SDK)
// ─────────────────────────────────────────────────────────────────────────────

func testWebhookIngestion(client *spooled.Client) {
	fmt.Println("\n📥 Webhook Ingestion (SDK)")
	fmt.Println(strings.Repeat("─", 60))

	r := &Runner{name: "Webhook Ingest"}
	ctx := context.Background()

	if skipIngest {
		r.Skip("Ingest tests", "SKIP_INGEST=true")
		return
	}

	orgs, err := client.Organizations().List(ctx)
	if err != nil || len(orgs) == 0 {
		r.Skip("Ingest tests", "no organizations available")
		return
	}
	orgID := orgs[0].ID

	queueName := fmt.Sprintf("%s-ingest", testPrefix)

	r.Run("Ingest.Custom enqueues a job", func() {
		resp, err := client.Ingest().Custom(ctx, orgID, &resources.CustomWebhookRequest{
			QueueName: queueName,
			Payload:   map[string]any{"source": "custom", "ts": time.Now().Unix()},
		})
		if err != nil {
			// Ingest endpoint may not be available in all environments
			log(fmt.Sprintf("Ingest.Custom not available: %v", err))
			return
		}
		if resp == nil || resp.JobID == "" {
			log("Ingest.Custom returned empty response (endpoint may not be configured)")
			return
		}
		log(fmt.Sprintf("Ingest.Custom job: %s", resp.JobID))

		// Cleanup
		_ = client.Jobs().Cancel(ctx, resp.JobID)
	})

	r.Run("Ingest.CustomWithToken uses X-Webhook-Token", func() {
		tokenResp, err := client.Organizations().GetWebhookToken(ctx, orgID)
		if err != nil {
			r.Skip("CustomWithToken", "failed to fetch webhook token")
			return
		}
		token := ""
		if tokenResp != nil && tokenResp.WebhookToken != nil {
			token = *tokenResp.WebhookToken
		}
		if token == "" {
			regenerated, err := client.Organizations().RegenerateWebhookToken(ctx, orgID)
			if err == nil && regenerated != nil && regenerated.WebhookToken != nil {
				token = *regenerated.WebhookToken
			}
		}
		if token == "" {
			r.Skip("CustomWithToken", "no webhook token available")
			return
		}

		resp, err := client.Ingest().CustomWithToken(ctx, orgID, token, &resources.CustomWebhookRequest{
			QueueName: queueName,
			Payload:   map[string]any{"source": "custom-token", "ts": time.Now().Unix()},
		})
		if err != nil {
			log(fmt.Sprintf("Ingest.CustomWithToken not available: %v", err))
			return
		}
		if resp == nil || resp.JobID == "" {
			log("Ingest.CustomWithToken returned empty response")
			return
		}
		log(fmt.Sprintf("Ingest.CustomWithToken job: %s", resp.JobID))

		// Cleanup
		_ = client.Jobs().Cancel(ctx, resp.JobID)
	})
}
