# Contributing to Spooled Go SDK

Thank you for your interest in contributing to the Spooled Go SDK! This document provides guidelines and instructions for contributing.

## Code of Conduct

By participating in this project, you agree to abide by our [Code of Conduct](https://github.com/spooled-cloud/spooled-backend/blob/main/CODE_OF_CONDUCT.md).

## Getting Started

### Prerequisites

- Go 1.25 or later
- Make (optional, for running scripts)
- Protocol Buffers compiler (for gRPC changes)

### Setup

1. Fork the repository
2. Clone your fork:
   ```bash
   git clone https://github.com/YOUR_USERNAME/spooled-sdk-go.git
   cd spooled-sdk-go
   ```
3. Add upstream remote:
   ```bash
   git remote add upstream https://github.com/spooled-cloud/spooled-sdk-go.git
   ```
4. Install dependencies:
   ```bash
   go mod download
   ```

## Development

### Project Structure

```
spooled-sdk-go/
├── examples/           # Runnable examples
├── internal/           # Internal packages (not exported)
│   ├── httpx/          # HTTP transport layer
│   └── version/        # Version information
├── scripts/            # Build and test scripts
├── spooled/            # Main SDK package
│   ├── grpc/           # gRPC client
│   ├── realtime/       # WebSocket/SSE clients
│   ├── resources/      # API resource implementations
│   ├── types/          # Shared type definitions
│   └── worker/         # Worker runtime
├── go.mod
├── README.md
└── CHANGELOG.md
```

### Running Tests

```bash
# Match the CI test command
go test -v -race -coverprofile=coverage.out -coverpkg=./spooled/...,./internal/... ./...

# Build all packages and examples
go build -v ./...
for dir in examples/*/; do (cd "$dir" && go build -v .); done

# Run the repository linter
golangci-lint run ./...

# Optional local equivalent of CI's non-blocking security scan
gosec ./...

# Run specific package tests
go test ./spooled/...

# Run integration tests (requires API key)
API_KEY=sp_test_... BASE_URL=http://localhost:8080 go run scripts/test-local/main.go
```

### Code Style

We follow standard Go conventions:

- Run `go fmt ./...` (and `goimports -w .`, as used by `make fmt`) before committing
- Run `go vet ./...` to check for issues
- Run `golangci-lint run ./...`, matching CI
- Follow [Effective Go](https://golang.org/doc/effective_go) guidelines
- Keep functions focused and well-documented

### Documentation

- All exported types, functions, and methods must have godoc comments
- Include examples in godoc where appropriate
- Update README.md for user-facing changes
- Update CHANGELOG.md for all changes

## Making Changes

### Branching

1. Create a feature branch from `main`:
   ```bash
   git checkout -b feature/your-feature-name
   ```

2. Make your changes with clear, atomic commits

3. Keep your branch up to date:
   ```bash
   git fetch upstream
   git rebase upstream/main
   ```

### Commit Messages

Follow [Conventional Commits](https://www.conventionalcommits.org/):

```
<type>(<scope>): <description>

[optional body]

[optional footer]
```

Types:
- `feat`: New feature
- `fix`: Bug fix
- `docs`: Documentation only
- `style`: Code style (formatting, etc.)
- `refactor`: Code refactoring
- `test`: Adding or updating tests
- `chore`: Maintenance tasks

Examples:
```
feat(jobs): add bulk create endpoint
fix(worker): handle graceful shutdown properly
docs(readme): update installation instructions
```

### Pull Requests

1. Ensure all tests pass
2. Update documentation if needed
3. Add entries to CHANGELOG.md
4. Create a pull request with a clear description
5. Link any related issues

### PR Checklist

- [ ] CI-equivalent tests pass (`go test -v -race -coverprofile=coverage.out -coverpkg=./spooled/...,./internal/... ./...`)
- [ ] Packages and examples build
- [ ] `golangci-lint run ./...` passes
- [ ] Security-sensitive changes were checked with `gosec ./...` (CI uploads a non-blocking SARIF report)
- [ ] Code is formatted (`go fmt ./...`; `goimports -w .` when available)
- [ ] No vet warnings (`go vet ./...`)
- [ ] Documentation updated
- [ ] CHANGELOG.md updated
- [ ] Commits follow conventions

## Release Process

Releases are managed by maintainers. This checklist records evidence without blocking ordinary commits, experiments, or cross-repository compatibility research. A mismatch between a Go module tag and this module's embedded runtime identity is a release error. Publishing a module is not a service deployment; consumers report new metadata only after upgrading and restarting or redeploying their applications.

Before pushing `vX.Y.Z`:

- [ ] Set `internal/version.Version` to `X.Y.Z`; User-Agent helpers must continue deriving from it.
- [ ] Confirm both high-level `spooled.SpooledWorker` and low-level `spooled/worker.Worker` default registration versions derive from `internal/version.Version`.
- [ ] Add an exact `## [X.Y.Z] - YYYY-MM-DD` section to `CHANGELOG.md` in descending release order.
- [ ] Review direct REST/gRPC registration examples so the top-level worker version is not confused with arbitrary metadata.
- [ ] Regenerate OpenAPI/protobuf code when its inputs change and record the proto commit/digest plus generator versions.
- [ ] Confirm no executable, coverage file, credential, or machine-specific build artifact is tracked or included in the module.
- [ ] Run the CI-equivalent race/coverage tests, `go vet ./...`, `golangci-lint run ./...`, `go build ./...`, and example builds.
- [ ] Record intentional dependency or cross-repository divergence with an owner, reason, evidence, review date, and exit condition; component versions need not match numerically.

Publish and verify:

- [ ] Create one immutable SemVer tag `vX.Y.Z` on the reviewed commit and never move or recreate it. If externally visible content is wrong, fix forward with a patch version and retract the bad version when appropriate.
- [ ] Record the tag commit and release workflow URL; the workflow validates strict SemVer, runtime version, and changelog identity before creating the GitHub Release.
- [ ] Verify `proxy.golang.org/.../@v/vX.Y.Z.info` identifies the intended commit and inspect the proxy `.mod` and module zip.
- [ ] Confirm the module zip contains the intended `internal/version.Version` and both worker default paths, with no local binaries or other unintended artifacts.
- [ ] Confirm `sum.golang.org` records the version and pkg.go.dev indexes it.
- [ ] Record module publication separately from consumer rollout, backend deployment, or documentation deployment.

## Getting Help

- Open an issue for bugs or feature requests
- Email support@spooled.cloud for private matters

## License

By contributing, you agree that your contributions will be licensed under the Apache 2.0 License.
