// Package modbus provides types and utilities for Modbus TCP/RTU communication.
package modbus

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/goburrow/modbus"
	"github.com/nexus-edge/protocol-gateway/internal/domain"
	"github.com/rs/zerolog"
)

// Client represents a Modbus client connection to a single device.
type Client struct {
	config              ClientConfig
	handler             *modbus.TCPClientHandler
	client              modbus.Client
	logger              zerolog.Logger
	mu                  sync.RWMutex
	opMu                sync.Mutex // Serializes all Modbus operations - goburrow client is NOT thread-safe
	connected           atomic.Bool
	lastError           error
	lastUsed            time.Time
	stats               *ClientStats
	deviceID            string
	consecutiveFailures atomic.Int32 // For backoff reset on success
	tagDiagnostics      sync.Map     // map[string]*TagDiagnostic - per-tag success/error tracking
}

// ClientConfig holds configuration for a Modbus client.
type ClientConfig struct {
	// Address is the host:port for TCP or serial port for RTU
	Address string

	// SlaveID is the Modbus slave/unit ID (1-247)
	SlaveID byte

	// Timeout is the connection and response timeout
	Timeout time.Duration

	// IdleTimeout is how long to keep idle connections open
	IdleTimeout time.Duration

	// MaxRetries is the number of retry attempts on transient failures
	MaxRetries int

	// RetryDelay is the base delay between retries (exponential backoff applied)
	RetryDelay time.Duration

	// Protocol specifies TCP or RTU
	Protocol domain.Protocol
}

// ClientStats tracks client performance metrics.
type ClientStats struct {
	ReadCount      atomic.Uint64
	WriteCount     atomic.Uint64
	ErrorCount     atomic.Uint64
	RetryCount     atomic.Uint64
	TotalReadTime  atomic.Int64 // nanoseconds
	TotalWriteTime atomic.Int64 // nanoseconds
}

// TagDiagnostic tracks per-tag success/error metrics.
type TagDiagnostic struct {
	TagID           string
	ReadCount       atomic.Uint64
	ErrorCount      atomic.Uint64
	LastError       atomic.Value // stores error
	LastErrorTime   atomic.Value // stores time.Time
	LastSuccessTime atomic.Value // stores time.Time
}

// NewTagDiagnostic creates a new tag diagnostic tracker.
func NewTagDiagnostic(tagID string) *TagDiagnostic {
	return &TagDiagnostic{
		TagID: tagID,
	}
}

// RegisterRange represents a contiguous range of registers to read in one operation.
type RegisterRange struct {
	StartAddress uint16
	EndAddress   uint16        // Inclusive end address
	Tags         []*domain.Tag // Tags within this range
}

// BatchConfig configures the range-based batching algorithm.
type BatchConfig struct {
	// MaxRegistersPerRead is the maximum registers per single Modbus read (protocol limit: 125)
	MaxRegistersPerRead uint16
	// MaxGapSize is the maximum gap between addresses to merge into one read
	// Higher values = fewer reads but more wasted bandwidth
	MaxGapSize uint16
}

// DefaultBatchConfig returns sensible defaults for batching.
func DefaultBatchConfig() BatchConfig {
	return BatchConfig{
		MaxRegistersPerRead: 100, // Conservative, well under 125 limit
		MaxGapSize:          10,  // Merge if gap <= 10 registers
	}
}

// PoolConfig holds configuration for the connection pool.
type PoolConfig struct {
	// MaxConnections is the maximum number of concurrent connections
	MaxConnections int

	// IdleTimeout is how long to keep idle connections open
	IdleTimeout time.Duration

	// HealthCheckPeriod is how often to check connection health
	HealthCheckPeriod time.Duration

	// ConnectionTimeout is the timeout for establishing new connections
	ConnectionTimeout time.Duration

	// RetryAttempts is the number of retry attempts for failed operations
	RetryAttempts int

	// RetryDelay is the base delay between retries
	RetryDelay time.Duration

	// CircuitBreakerName is the name for the circuit breaker
	CircuitBreakerName string
}

// DefaultPoolConfig returns a PoolConfig with sensible defaults.
// MaxConnections defaults to 500 to support industrial-scale deployments (100-1000 devices).
func DefaultPoolConfig() PoolConfig {
	return PoolConfig{
		MaxConnections:     500,
		IdleTimeout:        5 * time.Minute,
		HealthCheckPeriod:  30 * time.Second,
		ConnectionTimeout:  10 * time.Second,
		RetryAttempts:      3,
		RetryDelay:         100 * time.Millisecond,
		CircuitBreakerName: "modbus-pool",
	}
}

// PoolStats contains pool statistics.
type PoolStats struct {
	TotalConnections  int
	ActiveConnections int
	InUseConnections  int
	MaxConnections    int
}

// DeviceHealth contains health information for a single device.
type DeviceHealth struct {
	DeviceID           string
	Connected          bool
	CircuitBreakerOpen bool
	LastError          error
}

// DeviceStats contains statistics for a specific device.
type DeviceStats struct {
	DeviceID       string
	ReadCount      uint64
	WriteCount     uint64
	ErrorCount     uint64
	RetryCount     uint64
	AvgReadTimeMs  float64
	AvgWriteTimeMs float64
	Connected      bool
}
