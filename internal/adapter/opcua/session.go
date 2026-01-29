// Package opcua provides session and device binding management for OPC UA clients.
package opcua

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nexus-edge/protocol-gateway/internal/domain"
	"github.com/sony/gobreaker"
)

// =============================================================================
// Session & Device Binding Types
// =============================================================================

// pooledSession represents a shared OPC UA session for an endpoint.
// Multiple devices can share this session if they connect to the same endpoint.
type pooledSession struct {
	client          *Client
	endpointKey     string
	endpointURL     string
	breaker         *gobreaker.CircuitBreaker // Per-endpoint circuit breaker
	devices         map[string]*DeviceBinding // Devices using this session
	lastError       error
	connectFailures int
	nextReconnectAt time.Time
	mu              sync.Mutex

	// === Subscription Recovery ===
	subscriptionState *SubscriptionRecoveryState

	// === Subscription Activity Tracking ===
	// Prevents idle reaper from killing sessions with active subscriptions
	lastPublishTime        time.Time // Last time a subscription publish was received
	hasActiveSubscriptions bool      // Whether any subscriptions are active

	// === Per-Endpoint Load Shaping ===
	// Prevents one noisy endpoint from consuming all global capacity
	inFlight    atomic.Int64 // Current in-flight operations for this endpoint
	maxInFlight int64        // Per-endpoint cap (from config)
}

// DeviceBinding tracks a device's association with a session.
// Each device has its own circuit breaker for config/semantic errors,
// separate from the endpoint breaker for connection errors.
type DeviceBinding struct {
	DeviceID       string
	Device         *domain.Device
	EndpointKey    string
	MonitoredItems map[string]uint32         // tagID -> monitoredItemID for rebind
	LastSequenceNo uint32                    // For republish tracking
	breaker        *gobreaker.CircuitBreaker // Per-device breaker for config errors
	mu             sync.RWMutex
}

// SubscriptionRecoveryState tracks state needed for subscription recovery.
// This enables republish requests and monitored item rebind after reconnect.
type SubscriptionRecoveryState struct {
	SubscriptionID        uint32
	LastSequenceNumber    uint32
	MonitoredItems        map[uint32]*MonitoredItemState // clientHandle -> state
	LastAcknowledgedSeqNo uint32
	MissedSequenceNumbers []uint32 // For republish requests
	mu                    sync.RWMutex
}

// MonitoredItemState tracks state for a single monitored item.
type MonitoredItemState struct {
	ClientHandle     uint32
	MonitoredItemID  uint32
	NodeID           string
	TagID            string
	SamplingInterval time.Duration
	QueueSize        uint32
	LastValue        interface{}
	LastTimestamp    time.Time
}

// Legacy compatibility: pooledClient is now an alias for backward compatibility.
type pooledClient = pooledSession

// =============================================================================
// Session Methods
// =============================================================================

func (ps *pooledSession) canAttemptReconnect(now time.Time) bool {
	return ps.nextReconnectAt.IsZero() || !now.Before(ps.nextReconnectAt)
}

func (ps *pooledSession) recordConnectResult(now time.Time, err error, baseDelay time.Duration) {
	if err == nil {
		ps.lastError = nil
		ps.connectFailures = 0
		ps.nextReconnectAt = time.Time{}
		return
	}

	ps.lastError = err
	ps.connectFailures++

	// Default backoff base
	if baseDelay <= 0 {
		baseDelay = 1 * time.Second
	}
	if baseDelay < 1*time.Second {
		baseDelay = 1 * time.Second
	}

	// TooManySessions needs larger base delay
	if isTooManySessionsError(err) {
		baseDelay = 1 * time.Minute
	} else if contains(err.Error(), "EOF") {
		baseDelay = 30 * time.Second
	} else if baseDelay < 5*time.Second {
		baseDelay = 5 * time.Second
	}

	shift := ps.connectFailures - 1
	if shift > 6 {
		shift = 6
	}

	delay := baseDelay * time.Duration(1<<uint(shift))
	maxDelay := 5 * time.Minute
	if delay > maxDelay {
		delay = maxDelay
	}

	ps.nextReconnectAt = now.Add(delay)
}

// hasRecentActivity returns true if the session has recent activity that should prevent idle reaping.
// This includes read/write operations AND subscription publish messages.
func (ps *pooledSession) hasRecentActivity(idleTimeout time.Duration) bool {
	now := time.Now()

	// Check client's last used time (reads/writes)
	if now.Sub(ps.client.LastUsed()) < idleTimeout {
		return true
	}

	// Check subscription activity - don't kill sessions with active subscriptions
	if ps.hasActiveSubscriptions && now.Sub(ps.lastPublishTime) < idleTimeout {
		return true
	}

	return false
}

// recordPublishActivity updates the last publish time (called by subscription callbacks).
func (ps *pooledSession) recordPublishActivity() {
	ps.lastPublishTime = time.Now()
}

// =============================================================================
// Endpoint Key Generation
// =============================================================================

// endpointKey generates a unique key for session sharing.
// Devices connecting to the same endpoint (host+port+security+auth) share a session.
// This is the Kepware/Ignition pattern for scaling to 200+ devices.
func endpointKey(device *domain.Device, config PoolConfig) string {
	conn := device.Connection

	// Build endpoint URL
	endpointURL := conn.OPCEndpointURL
	if endpointURL == "" {
		endpointURL = fmt.Sprintf("opc.tcp://%s:%d", conn.Host, conn.Port)
	}

	// Security settings
	secPolicy := config.DefaultSecurityPolicy
	if conn.OPCSecurityPolicy != "" {
		secPolicy = conn.OPCSecurityPolicy
	}
	secMode := config.DefaultSecurityMode
	if conn.OPCSecurityMode != "" {
		secMode = conn.OPCSecurityMode
	}
	authMode := config.DefaultAuthMode
	if conn.OPCAuthMode != "" {
		authMode = conn.OPCAuthMode
	}

	// Auth credentials (hashed for security)
	authHash := ""
	if conn.OPCUsername != "" {
		h := sha256.Sum256([]byte(conn.OPCUsername + ":" + conn.OPCPassword))
		authHash = hex.EncodeToString(h[:8])
	}

	// Certificate identity - HASH CONTENTS, not filename!
	// This handles: cert rotation, same path/new cert, same cert/different path
	certHash := ""
	if conn.OPCCertFile != "" {
		if certContents, err := os.ReadFile(conn.OPCCertFile); err == nil {
			h := sha256.Sum256(certContents)
			certHash = hex.EncodeToString(h[:8])
		} else {
			// Fallback to path hash if file unreadable (will cause reconnect if cert changes)
			h := sha256.Sum256([]byte(conn.OPCCertFile))
			certHash = hex.EncodeToString(h[:8])
		}
	}

	return fmt.Sprintf("%s|%s|%s|%s|%s|%s", endpointURL, secPolicy, secMode, authMode, authHash, certHash)
}

// =============================================================================
// Error Classification for Two-Tier Circuit Breakers
// =============================================================================

// IsDeviceError returns true if the error is a device/config-level error
// that should trip the device breaker (not the endpoint breaker).
//
// Device breaker trips on:
//   - BadNodeIDUnknown, BadNodeIDInvalid
//   - BadUserAccessDenied, BadNotWritable
//   - BadTypeMismatch, BadOutOfRange
//   - BadAttributeIdInvalid
//   - BadBrowseNameInvalid
//
// These are config/semantic errors specific to one device's tag mappings,
// NOT infrastructure errors affecting all devices on the endpoint.
func IsDeviceError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()

	// Node ID errors (bad tag mapping)
	if contains(errStr, "BadNodeId") ||
		contains(errStr, "BadNodeID") ||
		contains(errStr, "node not found") ||
		contains(errStr, "unknown node") {
		return true
	}

	// Access/permission errors (device-specific security)
	if contains(errStr, "BadUserAccessDenied") ||
		contains(errStr, "BadNotWritable") ||
		contains(errStr, "BadNotReadable") ||
		contains(errStr, "access denied") {
		return true
	}

	// Type/conversion errors (bad tag config)
	if contains(errStr, "BadTypeMismatch") ||
		contains(errStr, "BadOutOfRange") ||
		contains(errStr, "BadDataEncodingInvalid") ||
		contains(errStr, "type mismatch") ||
		contains(errStr, "conversion") {
		return true
	}

	// Attribute errors (bad tag definition)
	if contains(errStr, "BadAttributeIdInvalid") ||
		contains(errStr, "BadIndexRangeInvalid") ||
		contains(errStr, "BadBrowseNameInvalid") {
		return true
	}

	// Write rejections (device-specific validation)
	if contains(errStr, "BadWriteNotSupported") ||
		contains(errStr, "write rejected") ||
		contains(errStr, "BadNoEntryExists") {
		return true
	}

	return false
}

// IsEndpointError returns true if the error is an endpoint/connection-level error
// that should trip the endpoint breaker (affecting all devices on that endpoint).
//
// Endpoint breaker trips on:
//   - Connection errors (timeout, refused, reset, EOF)
//   - Secure channel failures
//   - TooManySessions
//   - Server-wide errors
func IsEndpointError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()

	// Connection-level errors
	if contains(errStr, "connection") ||
		contains(errStr, "timeout") ||
		contains(errStr, "closed") ||
		contains(errStr, "refused") ||
		contains(errStr, "reset") ||
		contains(errStr, "broken pipe") ||
		contains(errStr, "no route to host") ||
		contains(errStr, "EOF") {
		return true
	}

	// Secure channel errors
	if contains(errStr, "SecureChannel") ||
		contains(errStr, "BadSecureChannel") ||
		contains(errStr, "certificate") {
		return true
	}

	// Server capacity errors
	if contains(errStr, "TooManySessions") ||
		contains(errStr, "BadTooManyOperations") ||
		contains(errStr, "BadServerTooBusy") {
		return true
	}

	// Session errors
	if contains(errStr, "BadSessionId") ||
		contains(errStr, "BadSessionClosed") ||
		contains(errStr, "session") {
		return true
	}

	return false
}
