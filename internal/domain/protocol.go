// Package domain contains core business entities.
package domain

import (
	"context"
	"sync"
)

// ProtocolPool defines the interface for protocol-specific connection pools.
// All protocol adapters (Modbus, OPC UA, S7) implement this interface.
type ProtocolPool interface {
	// ReadTags reads multiple tags from a device.
	ReadTags(ctx context.Context, device *Device, tags []*Tag) ([]*DataPoint, error)

	// ReadTag reads a single tag from a device.
	ReadTag(ctx context.Context, device *Device, tag *Tag) (*DataPoint, error)

	// WriteTag writes a value to a tag on a device.
	WriteTag(ctx context.Context, device *Device, tag *Tag, value interface{}) error

	// Close closes all connections in the pool.
	Close() error

	// HealthCheck performs a health check on the pool.
	HealthCheck(ctx context.Context) error
}

// ProtocolManager manages multiple protocol pools and routes operations
// to the appropriate pool based on device protocol.
// Thread-safe for concurrent access.
type ProtocolManager struct {
	pools map[Protocol]ProtocolPool
	mu    sync.RWMutex
}

// NewProtocolManager creates a new protocol manager.
func NewProtocolManager() *ProtocolManager {
	return &ProtocolManager{
		pools: make(map[Protocol]ProtocolPool),
	}
}

// RegisterPool registers a protocol pool. Thread-safe.
func (pm *ProtocolManager) RegisterPool(protocol Protocol, pool ProtocolPool) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.pools[protocol] = pool
}

// GetPool returns the pool for a given protocol. Thread-safe.
func (pm *ProtocolManager) GetPool(protocol Protocol) (ProtocolPool, bool) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	pool, exists := pm.pools[protocol]
	return pool, exists
}

// ReadTags routes read operations to the appropriate pool. Thread-safe.
func (pm *ProtocolManager) ReadTags(ctx context.Context, device *Device, tags []*Tag) ([]*DataPoint, error) {
	pool, exists := pm.GetPool(device.Protocol)
	if !exists {
		return nil, ErrProtocolNotSupported
	}
	return pool.ReadTags(ctx, device, tags)
}

// ReadTag routes read operation to the appropriate pool. Thread-safe.
func (pm *ProtocolManager) ReadTag(ctx context.Context, device *Device, tag *Tag) (*DataPoint, error) {
	pool, exists := pm.GetPool(device.Protocol)
	if !exists {
		return nil, ErrProtocolNotSupported
	}
	return pool.ReadTag(ctx, device, tag)
}

// WriteTag routes write operations to the appropriate pool. Thread-safe.
func (pm *ProtocolManager) WriteTag(ctx context.Context, device *Device, tag *Tag, value interface{}) error {
	pool, exists := pm.GetPool(device.Protocol)
	if !exists {
		return ErrProtocolNotSupported
	}
	return pool.WriteTag(ctx, device, tag, value)
}

// Close closes all protocol pools. Thread-safe.
func (pm *ProtocolManager) Close() error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	var lastErr error
	for protocol, pool := range pm.pools {
		if err := pool.Close(); err != nil {
			lastErr = err
		}
		delete(pm.pools, protocol)
	}
	return lastErr
}

// HealthCheck checks all pools. Thread-safe.
func (pm *ProtocolManager) HealthCheck(ctx context.Context) error {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	for _, pool := range pm.pools {
		if err := pool.HealthCheck(ctx); err != nil {
			return err
		}
	}
	return nil
}

// RegisteredProtocols returns all registered protocol types.
func (pm *ProtocolManager) RegisteredProtocols() []Protocol {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	protocols := make([]Protocol, 0, len(pm.pools))
	for p := range pm.pools {
		protocols = append(protocols, p)
	}
	return protocols
}
