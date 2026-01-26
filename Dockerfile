# Build stage
FROM golang:1.22-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata

# Set working directory
WORKDIR /build

# Copy go.mod first
COPY go.mod ./

# Copy source code (needed for go mod tidy to resolve imports)
COPY . .

# Download dependencies and generate go.sum
RUN go mod tidy && go mod download

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-w -s -X main.version=$(git describe --tags --always --dirty 2>/dev/null || echo 'dev')" \
    -o /build/protocol-gateway \
    ./cmd/gateway

# Final stage
FROM alpine:3.19

# Install runtime dependencies
RUN apk add --no-cache ca-certificates tzdata docker-cli

# Create non-root user
RUN addgroup -g 1000 gateway && \
    adduser -u 1000 -G gateway -s /bin/sh -D gateway

# Optional: allow access to /var/run/docker.sock when mounted (common gid=999)
RUN addgroup -g 999 docker || true && addgroup gateway docker || true

# Create necessary directories
RUN mkdir -p /app/config /app/web && chown -R gateway:gateway /app

WORKDIR /app

# Copy binary from builder
COPY --from=builder /build/protocol-gateway /app/

# Copy default config
COPY --from=builder /build/config/* /app/config/

# Copy web UI files
COPY --from=builder /build/web/* /app/web/

# Use non-root user
USER gateway

# Expose ports
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health/live || exit 1

# Run the application
ENTRYPOINT ["/app/protocol-gateway"]

