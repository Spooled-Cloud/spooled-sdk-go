# Contributing to Spooled Go SDK

Thank you for your interest in contributing to the Spooled Go SDK! This document provides guidelines and instructions for contributing.

## Code of Conduct

By participating in this project, you agree to abide by our [Code of Conduct](https://github.com/spooled-cloud/spooled-backend/blob/main/CODE_OF_CONDUCT.md).

## Getting Started

### Prerequisites

- Go 1.22 or later
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
# Run all tests
go test ./...

# Run tests with race detection
go test -race ./...

# Run tests with coverage
go test -cover ./...

# Run specific package tests
go test ./spooled/...

# Run integration tests (requires API key)
API_KEY=sp_test_... BASE_URL=http://localhost:8080 go run scripts/test-local/main.go
```

### Code Style

We follow standard Go conventions:

- Run `go fmt` before committing
- Run `go vet` to check for issues
- Use `golint` or `staticcheck` for additional linting
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

- [ ] Tests pass (`go test ./...`)
- [ ] No race conditions (`go test -race ./...`)
- [ ] Code is formatted (`go fmt ./...`)
- [ ] No vet warnings (`go vet ./...`)
- [ ] Documentation updated
- [ ] CHANGELOG.md updated
- [ ] Commits follow conventions

## Release Process

Releases are managed by maintainers. The process:

1. Update version in `internal/version/version.go`
2. Update CHANGELOG.md with release date
3. Create a git tag: `git tag v1.x.x`
4. Push tag: `git push origin v1.x.x`
5. GitHub Actions will create the release

## Getting Help

- Open an issue for bugs or feature requests
- Email support@spooled.cloud for private matters

## License

By contributing, you agree that your contributions will be licensed under the Apache 2.0 License.

