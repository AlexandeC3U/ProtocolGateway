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
	$(GOTEST) -v -race -short ./...

# Run tests with coverage
test-cover:
	@echo "Running tests with coverage..."
	$(GOTEST) -v -race -coverprofile=coverage.out -covermode=atomic ./...
	$(GO) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

# Run integration tests
test-integration:
	@echo "Running integration tests..."
	$(GOTEST) -v -race -tags=integration ./...

# Run benchmarks
bench:
	@echo "Running benchmarks..."
	$(GOTEST) -bench=. -benchmem ./...

# Lint the code
lint:
	@echo "Linting..."
	$(GOLINT) run ./...

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

# Help
help:
	@echo "Protocol Gateway - Available commands:"
	@echo ""
	@echo "  make build           - Build the binary"
	@echo "  make build-all       - Build for multiple platforms"
	@echo "  make run             - Build and run the application"
	@echo "  make dev             - Run with hot reload (requires air)"
	@echo "  make test            - Run tests"
	@echo "  make test-cover      - Run tests with coverage report"
	@echo "  make test-integration- Run integration tests"
	@echo "  make bench           - Run benchmarks"
	@echo "  make lint            - Lint the code (requires golangci-lint)"
	@echo "  make fmt             - Format the code"
	@echo "  make vet             - Run go vet"
	@echo "  make clean           - Clean build artifacts"
	@echo "  make deps            - Download dependencies"
	@echo "  make deps-update     - Update dependencies"
	@echo "  make generate        - Generate mocks"
	@echo "  make docker-build    - Build Docker image"
	@echo "  make docker-run      - Run Docker container"
	@echo "  make docker-dev-up   - Start development environment"
	@echo "  make docker-dev-down - Stop development environment"
	@echo "  make docker-dev-logs - View development logs"
	@echo "  make security        - Run security scan"
	@echo "  make vuln            - Check for vulnerabilities"
	@echo "  make help            - Show this help message"

