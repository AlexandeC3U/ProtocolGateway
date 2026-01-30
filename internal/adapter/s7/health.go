// Package s7 provides health and statistics for S7 connection pool.
package s7

import (
	"context"
	"time"

	"github.com/nexus-edge/protocol-gateway/internal/domain"
	"github.com/sony/gobreaker"
)

// =============================================================================
// Client Health & Diagnostics
// =============================================================================

// GetTagDiagnostic returns diagnostic information for a specific tag.
func (c *Client) GetTagDiagnostic(tagID string) (*TagDiagnostic, bool) {
	c.tagDiagMu.RLock()
	defer c.tagDiagMu.RUnlock()

	if c.tagDiagnostics == nil {
		return nil, false
	}
	diag, exists := c.tagDiagnostics[tagID]
	return diag, exists
}

// GetAllTagDiagnostics returns diagnostic information for all tags.
func (c *Client) GetAllTagDiagnostics() map[string]*TagDiagnostic {
	c.tagDiagMu.RLock()
	defer c.tagDiagMu.RUnlock()

	if c.tagDiagnostics == nil {
		return nil
	}

	// Return a copy to prevent external modification
	result := make(map[string]*TagDiagnostic, len(c.tagDiagnostics))
	for k, v := range c.tagDiagnostics {
		result[k] = v
	}
	return result
}

// recordTagSuccess records a successful operation for a tag.
func (c *Client) recordTagSuccess(tagID string) {
	c.tagDiagMu.Lock()
	if c.tagDiagnostics == nil {
		c.tagDiagnostics = make(map[string]*TagDiagnostic)
	}
	diag, exists := c.tagDiagnostics[tagID]
	if !exists {
		diag = &TagDiagnostic{TagID: tagID}
		c.tagDiagnostics[tagID] = diag
	}
	c.tagDiagMu.Unlock()

	diag.mu.Lock()
	diag.LastSuccessTime = time.Now()
	diag.SuccessCount++
	diag.mu.Unlock()
}

// recordTagError records an error for a tag.
func (c *Client) recordTagError(tagID string, err error) {
	c.tagDiagMu.Lock()
	if c.tagDiagnostics == nil {
		c.tagDiagnostics = make(map[string]*TagDiagnostic)
	}
	diag, exists := c.tagDiagnostics[tagID]
	if !exists {
		diag = &TagDiagnostic{TagID: tagID}
		c.tagDiagnostics[tagID] = diag
	}
	c.tagDiagMu.Unlock()

	diag.mu.Lock()
	diag.LastError = err
	diag.LastErrorTime = time.Now()
	diag.ErrorCount++
	diag.mu.Unlock()
}

// GetStats returns the client statistics as a map.
func (c *Client) GetStats() map[string]uint64 {
	return map[string]uint64{
		"read_count":      c.stats.ReadCount.Load(),
		"write_count":     c.stats.WriteCount.Load(),
		"error_count":     c.stats.ErrorCount.Load(),
		"retry_count":     c.stats.RetryCount.Load(),
		"reconnect_count": c.stats.ReconnectCount.Load(),
		"total_read_ns":   uint64(c.stats.TotalReadTime.Load()),
		"total_write_ns":  uint64(c.stats.TotalWriteTime.Load()),
	}
}

// LastUsed returns when the client was last used.
func (c *Client) LastUsed() time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lastUsed
}

// DeviceID returns the device ID this client is connected to.
func (c *Client) DeviceID() string {
	return c.deviceID
}

// LastError returns the last error encountered by this client.
func (c *Client) LastError() error {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lastError
}

// ConsecutiveFailures returns the current consecutive failure count.
func (c *Client) ConsecutiveFailures() int32 {
	return c.consecutiveFailures.Load()
}

// =============================================================================
// Pool Health & Statistics Types
// =============================================================================

// PoolStats contains pool statistics.
type PoolStats struct {
	TotalConnections  int
	ActiveConnections int
	MaxConnections    int
	Devices           []DeviceStats
}

// DeviceStats contains per-device statistics.
type DeviceStats struct {
	DeviceID            string
	Connected           bool
	LastUsed            time.Time
	LastError           string
	ReadCount           uint64
	WriteCount          uint64
	ErrorCount          uint64
	ReconnectCount      uint64
	BreakerState        string
	ConsecutiveFailures int32
	TagDiagnosticsCount int
}

// DeviceHealth contains health information for a single device.
type DeviceHealth struct {
	DeviceID            string
	Connected           bool
	CircuitBreakerOpen  bool
	LastError           error
	LastUsed            time.Time
	ConsecutiveFailures int32
}

// =============================================================================
// Pool Health Check Methods
// =============================================================================

// HealthCheck performs a health check on the pool.
// Implements the health.Checker interface.
func (p *Pool) HealthCheck(ctx context.Context) error {
	p.mu.RLock()
	defer p.mu.RUnlock()

	select {
	case <-p.stopChan:
		return domain.ErrServiceStopped
	default:
	}

	// Empty pool is considered healthy (no devices configured)
	if len(p.clients) == 0 {
		return nil
	}

	// Check if at least one client is connected
	for _, entry := range p.clients {
		if entry.client.IsConnected() {
			return nil
		}
	}

	return domain.ErrConnectionClosed
}

// GetStats returns pool statistics with detailed per-device info.
func (p *Pool) GetStats() PoolStats {
	p.mu.RLock()
	defer p.mu.RUnlock()

	stats := PoolStats{
		TotalConnections: len(p.clients),
		MaxConnections:   p.maxConnections,
		Devices:          make([]DeviceStats, 0, len(p.clients)),
	}

	for id, entry := range p.clients {
		if entry.client.IsConnected() {
			stats.ActiveConnections++
		}

		clientStats := entry.client.GetStats()
		lastErrStr := ""
		if entry.client.LastError() != nil {
			lastErrStr = entry.client.LastError().Error()
		}

		// Count tag diagnostics
		tagDiagCount := 0
		if diags := entry.client.GetAllTagDiagnostics(); diags != nil {
			tagDiagCount = len(diags)
		}

		stats.Devices = append(stats.Devices, DeviceStats{
			DeviceID:            id,
			Connected:           entry.client.IsConnected(),
			LastUsed:            entry.lastUse,
			LastError:           lastErrStr,
			ReadCount:           clientStats["read_count"],
			WriteCount:          clientStats["write_count"],
			ErrorCount:          clientStats["error_count"],
			ReconnectCount:      clientStats["reconnect_count"],
			BreakerState:        entry.breaker.State().String(),
			ConsecutiveFailures: entry.client.ConsecutiveFailures(),
			TagDiagnosticsCount: tagDiagCount,
		})
	}

	return stats
}

// GetDeviceHealth returns health information for a specific device.
func (p *Pool) GetDeviceHealth(deviceID string) (DeviceHealth, bool) {
	p.mu.RLock()
	entry, exists := p.clients[deviceID]
	p.mu.RUnlock()

	if !exists {
		return DeviceHealth{}, false
	}

	entry.mu.Lock()
	defer entry.mu.Unlock()

	return DeviceHealth{
		DeviceID:            deviceID,
		Connected:           entry.client.IsConnected(),
		CircuitBreakerOpen:  entry.breaker.State() == gobreaker.StateOpen,
		LastError:           entry.client.LastError(),
		LastUsed:            entry.lastUse,
		ConsecutiveFailures: entry.client.ConsecutiveFailures(),
	}, true
}

// GetAllDeviceHealth returns health information for all devices.
func (p *Pool) GetAllDeviceHealth() []DeviceHealth {
	p.mu.RLock()
	defer p.mu.RUnlock()

	healths := make([]DeviceHealth, 0, len(p.clients))
	for deviceID, entry := range p.clients {
		entry.mu.Lock()
		healths = append(healths, DeviceHealth{
			DeviceID:            deviceID,
			Connected:           entry.client.IsConnected(),
			CircuitBreakerOpen:  entry.breaker.State() == gobreaker.StateOpen,
			LastError:           entry.client.LastError(),
			LastUsed:            entry.lastUse,
			ConsecutiveFailures: entry.client.ConsecutiveFailures(),
		})
		entry.mu.Unlock()
	}

	return healths
}

// GetDeviceTagDiagnostics returns per-tag diagnostics for a specific device.
func (p *Pool) GetDeviceTagDiagnostics(deviceID string) (map[string]*TagDiagnostic, bool) {
	p.mu.RLock()
	entry, exists := p.clients[deviceID]
	p.mu.RUnlock()

	if !exists {
		return nil, false
	}

	return entry.client.GetAllTagDiagnostics(), true
}
