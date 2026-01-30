// Package s7 provides a production-grade Siemens S7 client implementation
// with connection management, bidirectional communication, and comprehensive error handling.
package s7

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/nexus-edge/protocol-gateway/internal/domain"
	"github.com/robinson/gos7"
	"github.com/rs/zerolog"
)

// =============================================================================
// Client Types
// =============================================================================

// Client represents an S7 client connection to a single PLC.
type Client struct {
	config              ClientConfig
	handler             *gos7.TCPClientHandler
	client              gos7.Client
	logger              zerolog.Logger
	mu                  sync.RWMutex
	opMu                sync.Mutex // Serializes S7 operations - gos7 client is NOT thread-safe
	connected           atomic.Bool
	lastError           error
	lastUsed            time.Time
	stats               *ClientStats
	deviceID            string
	consecutiveFailures atomic.Int32 // For backoff reset on success
	tagDiagnostics      map[string]*TagDiagnostic
	tagDiagMu           sync.RWMutex
}

// ClientConfig holds configuration for an S7 client.
type ClientConfig struct {
	// Address is the IP address of the PLC
	Address string

	// Port is the TCP port (default: 102 for ISO-on-TCP)
	Port int

	// Rack is the rack number of the PLC (usually 0)
	Rack int

	// Slot is the slot number of the CPU module
	// S7-300/400: usually 2, S7-1200/1500: usually 0 or 1
	Slot int

	// Timeout is the connection and response timeout
	Timeout time.Duration

	// IdleTimeout is how long to keep idle connections open
	IdleTimeout time.Duration

	// MaxRetries is the number of retry attempts on transient failures
	MaxRetries int

	// RetryDelay is the base delay between retries (exponential backoff applied)
	RetryDelay time.Duration

	// PDUSize is the maximum PDU size (default: 480)
	PDUSize int
}

// ClientStats tracks client performance metrics.
type ClientStats struct {
	ReadCount      atomic.Uint64
	WriteCount     atomic.Uint64
	ErrorCount     atomic.Uint64
	RetryCount     atomic.Uint64
	TotalReadTime  atomic.Int64 // nanoseconds
	TotalWriteTime atomic.Int64 // nanoseconds
	ReconnectCount atomic.Uint64
}

// TagDiagnostic tracks per-tag health information for troubleshooting.
type TagDiagnostic struct {
	TagID           string
	LastError       error
	LastErrorTime   time.Time
	LastSuccessTime time.Time
	ErrorCount      uint64
	SuccessCount    uint64
	mu              sync.RWMutex
}

// =============================================================================
// S7 Area Code Mapping
// =============================================================================

// S7AreaCode maps domain S7Area to gos7 area codes.
var S7AreaCode = map[domain.S7Area]int{
	domain.S7AreaDB: 0x84, // Data Blocks
	domain.S7AreaM:  0x83, // Merkers
	domain.S7AreaI:  0x81, // Inputs
	domain.S7AreaQ:  0x82, // Outputs
	domain.S7AreaT:  0x1D, // Timers
	domain.S7AreaC:  0x1C, // Counters
}

// =============================================================================
// Buffer Pool for Memory Efficiency
// =============================================================================

// BufferPool provides reusable byte buffers to reduce allocations under load.
// Uses sync.Pool for thread-safe buffer recycling.
var BufferPool = &bufferPool{
	pool1:  &sync.Pool{New: func() interface{} { return make([]byte, 1) }},
	pool2:  &sync.Pool{New: func() interface{} { return make([]byte, 2) }},
	pool4:  &sync.Pool{New: func() interface{} { return make([]byte, 4) }},
	pool8:  &sync.Pool{New: func() interface{} { return make([]byte, 8) }},
	pool16: &sync.Pool{New: func() interface{} { return make([]byte, 16) }},
}

type bufferPool struct {
	pool1  *sync.Pool
	pool2  *sync.Pool
	pool4  *sync.Pool
	pool8  *sync.Pool
	pool16 *sync.Pool
}

// Get retrieves a buffer of at least the specified size from the pool.
func (bp *bufferPool) Get(size int) []byte {
	switch {
	case size <= 1:
		return bp.pool1.Get().([]byte)[:size]
	case size <= 2:
		return bp.pool2.Get().([]byte)[:size]
	case size <= 4:
		return bp.pool4.Get().([]byte)[:size]
	case size <= 8:
		return bp.pool8.Get().([]byte)[:size]
	case size <= 16:
		return bp.pool16.Get().([]byte)[:size]
	default:
		// For larger buffers, allocate directly
		return make([]byte, size)
	}
}

// Put returns a buffer to the pool for reuse.
func (bp *bufferPool) Put(buf []byte) {
	size := cap(buf)
	switch {
	case size == 1:
		bp.pool1.Put(buf[:1])
	case size == 2:
		bp.pool2.Put(buf[:2])
	case size == 4:
		bp.pool4.Put(buf[:4])
	case size == 8:
		bp.pool8.Put(buf[:8])
	case size == 16:
		bp.pool16.Put(buf[:16])
		// Larger buffers are not pooled, let GC handle them
	}
}
