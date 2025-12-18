// Production test runner for scripts/test-local/main.go.
//
// Safe defaults:
// - Uses Spooled Cloud endpoints
// - Skips stress/load tests unless --full is provided (to avoid noisy prod runs)
//
// Usage:
//   API_KEY=sp_live_... go run scripts/test-production.go
//
// Options:
//   --full     Run stress/load tests too (NOT recommended for prod)
//   --verbose  Enable verbose output in test-local.go
//
// Overrides (optional):
//   BASE_URL=https://api.spooled.cloud
//   GRPC_ADDRESS=grpc.spooled.cloud:443
//   SKIP_GRPC=0|1
//   SKIP_STRESS=0|1

package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func main() {
	args := os.Args[1:]
	isFull := contains(args, "--full")
	isVerbose := contains(args, "--verbose")

	apiKey := os.Getenv("API_KEY")
	if apiKey == "" {
		fmt.Fprintf(os.Stderr, "❌ API_KEY is required\n")
		fmt.Fprintf(os.Stderr, "   Example: API_KEY=sp_live_... go run scripts/test-production.go\n")
		os.Exit(1)
	}

	baseUrl := getEnv("BASE_URL", "https://api.spooled.cloud")
	grpcAddress := getEnv("GRPC_ADDRESS", "grpc.spooled.cloud:443")

	// Default to running gRPC tests in production (can be overridden)
	skipGrpc := getEnv("SKIP_GRPC", "0")
	// Default to skipping stress tests in production (can be overridden or --full)
	skipStress := "1"
	if isFull {
		skipStress = "0"
	}
	if env := os.Getenv("SKIP_STRESS"); env != "" {
		skipStress = env
	}

	// Build environment variables
	env := os.Environ()
	env = append(env,
		"BASE_URL="+baseUrl,
		"GRPC_ADDRESS="+grpcAddress,
		"SKIP_GRPC="+skipGrpc,
		"SKIP_STRESS="+skipStress,
	)

	if isVerbose {
		env = append(env, "VERBOSE=1")
	}

	fmt.Printf("🚀 Running Go SDK Production Tests\n")
	fmt.Printf("   API: %s\n", baseUrl)
	fmt.Printf("   gRPC: %s\n", grpcAddress)
	fmt.Printf("   Skip gRPC: %s\n", skipGrpc)
	fmt.Printf("   Skip Stress: %s\n", skipStress)
	fmt.Printf("   Verbose: %v\n", isVerbose)
	fmt.Println()

	// Run test-local/main.go from the scripts directory
	cmd := exec.Command("go", "run", "scripts/test-local/main.go")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = env

	// Change to parent directory if we're in scripts
	if strings.HasSuffix(os.Getenv("PWD"), "/scripts") {
		cmd.Dir = ".."
	}

	err := cmd.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "\n❌ Production tests failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("\n✅ Production tests passed!")
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}
