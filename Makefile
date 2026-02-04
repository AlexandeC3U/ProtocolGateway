# Protocol Gateway Makefile
# Production-grade build and development commands

.PHONY: all build run test test-cover lint fmt vet clean docker-build docker-run dev help

# Variables
BINARY_NAME=protocol-gateway
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME=$(shell date -u '+%Y-%m-%d_%H:%M:%S')
LDFLAGS=-ldflags "-w -s -X main.version=$(VERSION) -X main.buildTime=$(BUILD_TIME)"
GO=go
GOTEST=$(GO) test
GOVET=$(GO) vet
GOFMT=gofmt
GOLINT=golangci-lint

# Default target
all: lint test build

# Build the binary
build:
	@echo "Building $(BINARY_NAME)..."
	CGO_ENABLED=0 $(GO) build $(LDFLAGS) -o bin/$(BINARY_NAME) ./cmd/gateway

# Build for multiple platforms
build-all:
	@echo "Building for multiple platforms..."
	GOOS=linux GOARCH=amd64 $(GO) build $(LDFLAGS) -o bin/$(BINARY_NAME)-linux-amd64 ./cmd/gateway
	GOOS=linux GOARCH=arm64 $(GO) build $(LDFLAGS) -o bin/$(BINARY_NAME)-linux-arm64 ./cmd/gateway
	GOOS=darwin GOARCH=amd64 $(GO) build $(LDFLAGS) -o bin/$(BINARY_NAME)-darwin-amd64 ./cmd/gateway
	GOOS=darwin GOARCH=arm64 $(GO) build $(LDFLAGS) -o bin/$(BINARY_NAME)-darwin-arm64 ./cmd/gateway
	GOOS=windows GOARCH=amd64 $(GO) build $(LDFLAGS) -o bin/$(BINARY_NAME)-windows-amd64.exe ./cmd/gateway

# Run the application
run: build
	./bin/$(BINARY_NAME)

# Run with hot reload (requires air: go install github.com/cosmtrek/air@latest)
dev:
	air

# Run tests
test:
	@echo "Running tests..."
	$(GOTEST) -v -short ./...

# Run tests with race detection (requires CGO_ENABLED=1)
test-race:
	@echo "Running tests with race detection..."
	CGO_ENABLED=1 $(GOTEST) -v -race -short ./...

# Run tests with coverage
test-cover:
	@echo "Running tests with coverage..."
	$(GOTEST) -v -coverprofile=coverage.out -covermode=atomic ./...
	$(GO) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

# Run integration tests
test-integration:
	@echo "Running integration tests..."
	$(GOTEST) -v -tags=integration ./...

# Run benchmarks
bench:
	@echo "Running benchmarks..."
	$(GOTEST) -bench=. -benchmem -tags=benchmark ./...

# Run benchmarks with comparison output
bench-compare:
	@echo "Running benchmarks with comparison..."
	$(GOTEST) -bench=. -benchmem -tags=benchmark -count=5 ./... | tee bench.txt

# Lint the code
lint:
	@echo "Linting..."
	$(GOLINT) run ./...

# =============================================================================
# Extended Test Targets
# =============================================================================

# Run unit tests only (from testing/unit/)
test-unit:
	@echo "Running unit tests..."
	$(GOTEST) -v -race ./testing/unit/...

# Run all tests with verbose output
test-verbose:
	@echo "Running all tests (verbose)..."
	$(GOTEST) -v -race ./...

# Run tests for a specific package
test-pkg:
	@echo "Running tests for package: $(PKG)"
	$(GOTEST) -v -race ./$(PKG)/...

# Run fuzz tests (seed corpus validation - runs all fuzz targets once)
test-fuzz:
	@echo "Running fuzz tests (seed corpus validation)..."
	$(GOTEST) -tags=fuzz -v ./testing/fuzz/...

# Alias for fuzz tests
fuzz: test-fuzz

# Run a specific fuzz test for 30s (usage: make fuzz-target TARGET=FuzzModbusAddress)
fuzz-target:
	@echo "Running fuzz target $(TARGET) for 30s..."
	$(GOTEST) -tags=fuzz -fuzz=$(TARGET) -fuzztime=30s ./testing/fuzz/...

# Run fuzz tests (extended - one target at a time)
test-fuzz-long:
	@echo "Running FuzzReorderBytes_4Byte for 1m..."
	$(GOTEST) -tags=fuzz -fuzz=FuzzReorderBytes_4Byte -fuzztime=1m ./testing/fuzz/conversion/
	@echo "Running FuzzModbusAddress for 1m..."
	$(GOTEST) -tags=fuzz -fuzz=FuzzModbusAddress -fuzztime=1m ./testing/fuzz/parsing/

# Run end-to-end tests
test-e2e:
	@echo "Running end-to-end tests..."
	$(GOTEST) -v -tags=e2e ./testing/e2e/...

# Alias for e2e tests
e2e: test-e2e

# Run tests with coverage and generate HTML report
test-coverage-html:
	@echo "Generating coverage report..."
	$(GOTEST) -coverprofile=coverage.out -covermode=atomic ./...
	$(GO) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

# Run tests with coverage for specific package
test-coverage-pkg:
	@echo "Running coverage for package: $(PKG)"
	$(GOTEST) -coverprofile=coverage.out -covermode=atomic ./$(PKG)/...
	$(GO) tool cover -html=coverage.out -o coverage.html

# =============================================================================
# Test Environment Management
# =============================================================================

# Start test simulators (Modbus, OPC UA, S7, MQTT)
test-env-up:
	@echo "Starting test environment..."
	docker-compose -f docker-compose.test.yaml up -d
	@echo "Waiting for services to be ready..."
	@sleep 5
	@echo "Test environment ready!"

# Stop test simulators
test-env-down:
	@echo "Stopping test environment..."
	docker-compose -f docker-compose.test.yaml down

# View test environment logs
test-env-logs:
	docker-compose -f docker-compose.test.yaml logs -f

# Check test environment health
test-env-health:
	@echo "Checking test environment health..."
	docker-compose -f docker-compose.test.yaml ps

# Run full test suite (unit + integration)
test-all: test-env-up test test-integration test-env-down
	@echo "Full test suite completed!"

# =============================================================================
# Benchmark Targets
# =============================================================================

# Run throughput benchmarks
bench-throughput:
	@echo "Running throughput benchmarks..."
	$(GOTEST) -bench=. -benchmem ./testing/benchmark/throughput/...

# Run concurrency benchmarks
bench-concurrency:
	@echo "Running concurrency benchmarks..."
	$(GOTEST) -bench=. -benchmem ./testing/benchmark/concurrency/...

# Run memory benchmarks
bench-memory:
	@echo "Running memory benchmarks..."
	$(GOTEST) -bench=. -benchmem -memprofile=mem.out ./testing/benchmark/memory/...
	$(GO) tool pprof -text mem.out

# Run latency benchmarks
bench-latency:
	@echo "Running latency benchmarks..."
	$(GOTEST) -bench=. -benchmem ./testing/benchmark/latency/...

# =============================================================================
# Original Targets (continued)
# =============================================================================

# Format the code
fmt:
	@echo "Formatting..."
	$(GOFMT) -s -w .

# Run go vet
vet:
	@echo "Vetting..."
	$(GOVET) ./...

# Clean build artifacts
clean:
	@echo "Cleaning..."
	rm -rf bin/
	rm -f coverage.out coverage.html

# Download dependencies
deps:
	@echo "Downloading dependencies..."
	$(GO) mod download
	$(GO) mod tidy

# Update dependencies
deps-update:
	@echo "Updating dependencies..."
	$(GO) get -u ./...
	$(GO) mod tidy

# Generate mocks (requires mockgen: go install github.com/golang/mock/mockgen@latest)
generate:
	@echo "Generating mocks..."
	$(GO) generate ./...

# Docker build
docker-build:
	@echo "Building Docker image..."
	docker build -t nexus/protocol-gateway:$(VERSION) -t nexus/protocol-gateway:latest .

# Docker run
docker-run:
	docker run --rm -p 8080:8080 nexus/protocol-gateway:latest

# Start development environment with docker-compose
docker-dev-up:
	@echo "Starting development environment..."
	docker-compose -f docker-compose.dev.yaml up -d

# Stop development environment
docker-dev-down:
	@echo "Stopping development environment..."
	docker-compose -f docker-compose.dev.yaml down

# View development logs
docker-dev-logs:
	docker-compose -f docker-compose.dev.yaml logs -f

# Security scan (requires gosec: go install github.com/securego/gosec/v2/cmd/gosec@latest)
security:
	@echo "Running security scan..."
	gosec -quiet ./...

# Check for vulnerabilities (requires govulncheck: go install golang.org/x/vuln/cmd/govulncheck@latest)
vuln:
	@echo "Checking for vulnerabilities..."
	govulncheck ./...

# =============================================================================
# Git Helpers
# =============================================================================

# Git add all changes
git-add:
	@echo "Staging all changes..."
	git add -A

# Git commit with auto-generated message (simple version for cross-platform)
git-commit: git-add
	@echo "Committing changes..."
	git commit -m "Update project files"

# Git commit with custom message (usage: make git-commit-msg MSG="your message")
git-commit-msg: git-add
	@echo "Committing with message: $(MSG)"
	git commit -m "$(MSG)"

# Git push to current branch
git-push:
	@echo "Pushing to remote..."
	git push

# Git commit and push (auto message)
git-save: git-commit git-push
	@echo "Changes saved and pushed!"

# Git commit and push with custom message (usage: make git-save-msg MSG="your message")
git-save-msg: git-commit-msg git-push
	@echo "Changes saved and pushed!"

# Git status
git-status:
	@git status -s

# Help
help:
	@echo "Protocol Gateway - Available commands:"
	@echo ""
	@echo "Build & Run:"
	@echo "  make build              - Build the binary"
	@echo "  make build-all          - Build for multiple platforms"
	@echo "  make run                - Build and run the application"
	@echo "  make dev                - Run with hot reload (requires air)"
	@echo ""
	@echo "Testing:"
	@echo "  make test               - Run all tests (short mode)"
	@echo "  make test-unit          - Run unit tests only"
	@echo "  make test-verbose       - Run tests with verbose output"
	@echo "  make test-cover         - Run tests with coverage report"
	@echo "  make test-coverage-html - Generate HTML coverage report"
	@echo "  make test-integration   - Run integration tests"
	@echo "  make test-e2e           - Run end-to-end tests"
	@echo "  make test-fuzz          - Run fuzz tests (30s)"
	@echo "  make test-race          - Run tests with race detector"
	@echo "  make test-all           - Run full test suite"
	@echo ""
	@echo "Benchmarks:"
	@echo "  make bench              - Run all benchmarks"
	@echo "  make bench-throughput   - Run throughput benchmarks"
	@echo "  make bench-concurrency  - Run concurrency benchmarks"
	@echo "  make bench-memory       - Run memory benchmarks"
	@echo "  make bench-latency      - Run latency benchmarks"
	@echo ""
	@echo "Test Environment:"
	@echo "  make test-env-up        - Start test simulators"
	@echo "  make test-env-down      - Stop test simulators"
	@echo "  make test-env-logs      - View simulator logs"
	@echo "  make test-env-health    - Check simulator health"
	@echo ""
	@echo "Code Quality:"
	@echo "  make lint               - Lint the code (requires golangci-lint)"
	@echo "  make fmt                - Format the code"
	@echo "  make vet                - Run go vet"
	@echo "  make security           - Run security scan"
	@echo "  make vuln               - Check for vulnerabilities"
	@echo ""
	@echo "Dependencies:"
	@echo "  make deps               - Download dependencies"
	@echo "  make deps-update        - Update dependencies"
	@echo "  make generate           - Generate mocks"
	@echo ""
	@echo "Docker:"
	@echo "  make docker-build       - Build Docker image"
	@echo "  make docker-run         - Run Docker container"
	@echo "  make docker-dev-up      - Start development environment"
	@echo "  make docker-dev-down    - Stop development environment"
	@echo "  make clean              - Clean build artifacts"
	@echo ""
	@echo "Git Helpers:"
	@echo "  make git-status         - Show git status"
	@echo "  make git-save           - Add, commit (auto msg), and push"
	@echo "  make git-save-msg MSG=\"message\" - Commit with custom message and push"
	@echo "  make git-commit         - Add and commit with auto message"
	@echo "  make git-commit-msg MSG=\"message\" - Commit with custom message"
	@echo "  make git-push           - Push to remote"
	@echo ""

