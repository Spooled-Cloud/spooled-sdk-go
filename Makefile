# Spooled Go SDK Makefile

.PHONY: all build test lint fmt clean generate examples help

# Default target
all: lint test build

# Build the SDK
build:
	@echo "Building..."
	go build -v ./...

# Run tests
test:
	@echo "Running tests..."
	go test -v -race -cover ./...

# Run tests with coverage report
test-coverage:
	@echo "Running tests with coverage..."
	go test -v -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

# Run linter
lint:
	@echo "Running linter..."
	golangci-lint run ./...

# Format code
fmt:
	@echo "Formatting code..."
	go fmt ./...
	goimports -w .

# Clean build artifacts
clean:
	@echo "Cleaning..."
	rm -f coverage.out coverage.html
	rm -f main test-local
	go clean ./...

# Generate protobuf files
generate-proto:
	@echo "Generating protobuf files..."
	./scripts/generate_grpc.sh

# Generate OpenAPI types
generate-openapi:
	@echo "Generating OpenAPI types..."
	./scripts/generate_openapi.sh

# Generate all
generate: generate-proto generate-openapi

# Run integration tests (requires API_KEY)
integration-test:
	@echo "Running integration tests..."
	@if [ -z "$(API_KEY)" ]; then \
		echo "Error: API_KEY environment variable required"; \
		exit 1; \
	fi
	go run scripts/test-local/main.go

# Build and run all examples
examples:
	@echo "Building examples..."
	@for dir in examples/*/; do \
		echo "Building $$dir..."; \
		(cd "$$dir" && go build -v .); \
	done

# Run quick-start example
example-quick-start:
	@echo "Running quick-start example..."
	@if [ -z "$(API_KEY)" ]; then \
		echo "Error: API_KEY environment variable required"; \
		exit 1; \
	fi
	go run examples/quick-start/main.go

# Run worker example
example-worker:
	@echo "Running worker example..."
	@if [ -z "$(API_KEY)" ]; then \
		echo "Error: API_KEY environment variable required"; \
		exit 1; \
	fi
	go run examples/worker/main.go

# Run grpc example
example-grpc:
	@echo "Running gRPC example..."
	@if [ -z "$(API_KEY)" ]; then \
		echo "Error: API_KEY environment variable required"; \
		exit 1; \
	fi
	go run examples/grpc/main.go

# Tidy dependencies
tidy:
	@echo "Tidying dependencies..."
	go mod tidy

# Verify dependencies
verify:
	@echo "Verifying dependencies..."
	go mod verify

# Security scan
security:
	@echo "Running security scan..."
	gosec ./...

# Documentation
docs:
	@echo "Opening documentation..."
	godoc -http=:6060 &
	@echo "Documentation server running at http://localhost:6060/pkg/github.com/spooled-cloud/spooled-sdk-go/"

# Help
help:
	@echo "Spooled Go SDK Makefile"
	@echo ""
	@echo "Usage:"
	@echo "  make [target]"
	@echo ""
	@echo "Targets:"
	@echo "  all              Build, lint, and test (default)"
	@echo "  build            Build the SDK"
	@echo "  test             Run unit tests"
	@echo "  test-coverage    Run tests with coverage report"
	@echo "  lint             Run linter"
	@echo "  fmt              Format code"
	@echo "  clean            Clean build artifacts"
	@echo "  generate         Generate protobuf and OpenAPI types"
	@echo "  integration-test Run integration tests (requires API_KEY)"
	@echo "  examples         Build all examples"
	@echo "  example-*        Run specific example (requires API_KEY)"
	@echo "  tidy             Tidy dependencies"
	@echo "  verify           Verify dependencies"
	@echo "  security         Run security scan"
	@echo "  docs             Start local documentation server"
	@echo "  help             Show this help"

