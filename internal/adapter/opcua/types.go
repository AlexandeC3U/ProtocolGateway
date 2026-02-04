// Package opcua provides type definitions for OPC UA communication.
package opcua

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/gopcua/opcua"
	"github.com/gopcua/opcua/ua"
	"github.com/nexus-edge/protocol-gateway/internal/domain"
	"github.com/rs/zerolog"
)

// =============================================================================
// Client Types (moved from client.go for consistency with Modbus/S7)
// =============================================================================

// SessionState represents the OPC UA session state machine.
type SessionState string

const (
	SessionStateDisconnected  SessionState = "disconnected"
	SessionStateConnecting    SessionState = "connecting"
	SessionStateSecureChannel SessionState = "secure_channel"
	SessionStateActive        SessionState = "active"
	SessionStateError         SessionState = "error"
)

// Client represents an OPC UA client connection to a single server.
type Client struct {
	config       ClientConfig
	client       *opcua.Client
	logger       zerolog.Logger
	mu           sync.RWMutex
	opMu         sync.Mutex // Serializes OPC UA operations for thread safety
	connected    atomic.Bool
	sessionState SessionState
	lastError    error
	lastUsed     time.Time
	stats        *ClientStats
	deviceID     string
	nodeCache    map[string]*ua.NodeID // Cache parsed node IDs
	nodeCacheMu  sync.RWMutex

	// Consecutive failures for backoff reset
	consecutiveFailures atomic.Int32

	// Per-tag diagnostic tracking
	tagDiagnostics sync.Map // map[string]*TagDiagnostic
}

// ClientConfig holds configuration for an OPC UA client.
type ClientConfig struct {
	// EndpointURL is the OPC UA server endpoint (e.g., "opc.tcp://localhost:4840")
	EndpointURL string

	// SecurityPolicy specifies the security policy (None, Basic128Rsa15, Basic256, Basic256Sha256)
	SecurityPolicy string

	// SecurityMode specifies the security mode (None, Sign, SignAndEncrypt)
	SecurityMode string

	// AuthMode specifies authentication mode (Anonymous, UserName, Certificate)
	AuthMode string

	// Username for UserName authentication
	Username string

	// Password for UserName authentication
	Password string

	// CertificateFile path for client certificate (PEM or DER format)
	CertificateFile string

	// PrivateKeyFile path for client private key (PEM format)
	PrivateKeyFile string

	// ServerCertificateFile path for trusted server certificate (optional)
	// If not provided, the client will trust any server certificate
	ServerCertificateFile string

	// InsecureSkipVerify disables server certificate verification
	// WARNING: Only use for testing or when server cert is self-signed
	InsecureSkipVerify bool

	// AutoSelectEndpoint enables automatic endpoint discovery and security selection
	// When true, the client queries available endpoints and selects the best match
	AutoSelectEndpoint bool

	// ApplicationName is the OPC UA application name for this client
	ApplicationName string

	// ApplicationURI is the OPC UA application URI for this client
	ApplicationURI string

	// Timeout is the connection and response timeout
	Timeout time.Duration

	// KeepAlive is the keep-alive interval
	KeepAlive time.Duration

	// MaxRetries is the number of retry attempts on transient failures
	MaxRetries int

	// RetryDelay is the base delay between retries (exponential backoff applied)
	RetryDelay time.Duration

	// RequestTimeout is the timeout for individual requests
	RequestTimeout time.Duration

	// SessionTimeout is the session timeout on the server
	SessionTimeout time.Duration

	// === Subscription Settings ===

	// DefaultPublishingInterval is the default subscription publishing interval
	DefaultPublishingInterval time.Duration

	// DefaultSamplingInterval is the default monitored item sampling interval
	DefaultSamplingInterval time.Duration

	// DefaultQueueSize is the default monitored item queue size
	DefaultQueueSize uint32
}

// ClientStats tracks client performance metrics.
type ClientStats struct {
	ReadCount         atomic.Uint64
	WriteCount        atomic.Uint64
	ErrorCount        atomic.Uint64
	RetryCount        atomic.Uint64
	SubscribeCount    atomic.Uint64
	NotificationCount atomic.Uint64
	TotalReadTime     atomic.Int64 // nanoseconds
	TotalWriteTime    atomic.Int64 // nanoseconds
}

// TagDiagnostic tracks per-tag success/error metrics.
type TagDiagnostic struct {
	TagID           string
	NodeID          string
	ReadCount       atomic.Uint64
	ErrorCount      atomic.Uint64
	LastError       atomic.Value // stores error
	LastErrorTime   atomic.Value // stores time.Time
	LastSuccessTime atomic.Value // stores time.Time
}

// NewTagDiagnostic creates a new tag diagnostic tracker.
func NewTagDiagnostic(tagID, nodeID string) *TagDiagnostic {
	return &TagDiagnostic{
		TagID:  tagID,
		NodeID: nodeID,
	}
}

// DeviceStats contains statistics for a specific device.
type DeviceStats struct {
	DeviceID          string
	ReadCount         uint64
	WriteCount        uint64
	ErrorCount        uint64
	RetryCount        uint64
	SubscribeCount    uint64
	NotificationCount uint64
	AvgReadTimeMs     float64
	AvgWriteTimeMs    float64
	Connected         bool
	SessionState      SessionState
}

// =============================================================================
// OPC UA Specific Types
// =============================================================================

// NodeInfo contains metadata about an OPC UA node.
type NodeInfo struct {
	NodeID      *ua.NodeID
	DisplayName string
	DataType    ua.TypeID
	AccessLevel ua.AccessLevelType
}

// WriteRequest represents a write operation to an OPC UA node.
type WriteRequest struct {
	NodeID string
	Value  interface{}
	Tag    *domain.Tag
}

// BrowseResult contains the result of browsing an OPC UA node.
type BrowseResult struct {
	NodeID      string
	DisplayName string
	NodeClass   ua.NodeClass
	Children    []*BrowseResult
}
