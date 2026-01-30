// Package modbus provides health monitoring and diagnostics for Modbus connections.
package modbus

import (
	"time"
)

// GetTagDiagnostic returns diagnostic information for a specific tag.
func (c *Client) GetTagDiagnostic(tagID string) *TagDiagnostic {
	if diag, ok := c.tagDiagnostics.Load(tagID); ok {
		return diag.(*TagDiagnostic)
	}
	return nil
}

// GetAllTagDiagnostics returns diagnostics for all tags.
func (c *Client) GetAllTagDiagnostics() map[string]*TagDiagnostic {
	result := make(map[string]*TagDiagnostic)
	c.tagDiagnostics.Range(func(key, value interface{}) bool {
		result[key.(string)] = value.(*TagDiagnostic)
		return true
	})
	return result
}

// recordTagSuccess records a successful read for a tag.
func (c *Client) recordTagSuccess(tagID string) {
	diag := c.getOrCreateTagDiagnostic(tagID)
	diag.ReadCount.Add(1)
	diag.LastSuccessTime.Store(time.Now())
}

// recordTagError records a read error for a tag.
func (c *Client) recordTagError(tagID string, err error) {
	diag := c.getOrCreateTagDiagnostic(tagID)
	diag.ErrorCount.Add(1)
	diag.LastError.Store(err)
	diag.LastErrorTime.Store(time.Now())
}

// getOrCreateTagDiagnostic gets or creates a diagnostic tracker for a tag.
func (c *Client) getOrCreateTagDiagnostic(tagID string) *TagDiagnostic {
	if diag, ok := c.tagDiagnostics.Load(tagID); ok {
		return diag.(*TagDiagnostic)
	}
	diag := NewTagDiagnostic(tagID)
	actual, _ := c.tagDiagnostics.LoadOrStore(tagID, diag)
	return actual.(*TagDiagnostic)
}

// GetDeviceStats returns detailed statistics for this client.
func (c *Client) GetDeviceStats() DeviceStats {
	readCount := c.stats.ReadCount.Load()
	writeCount := c.stats.WriteCount.Load()
	totalReadNs := c.stats.TotalReadTime.Load()
	totalWriteNs := c.stats.TotalWriteTime.Load()

	var avgReadMs, avgWriteMs float64
	if readCount > 0 {
		avgReadMs = float64(totalReadNs) / float64(readCount) / 1e6
	}
	if writeCount > 0 {
		avgWriteMs = float64(totalWriteNs) / float64(writeCount) / 1e6
	}

	return DeviceStats{
		DeviceID:       c.deviceID,
		ReadCount:      readCount,
		WriteCount:     writeCount,
		ErrorCount:     c.stats.ErrorCount.Load(),
		RetryCount:     c.stats.RetryCount.Load(),
		AvgReadTimeMs:  avgReadMs,
		AvgWriteTimeMs: avgWriteMs,
		Connected:      c.connected.Load(),
	}
}

// GetStats returns the client statistics as a map.
func (c *Client) GetStats() map[string]uint64 {
	return map[string]uint64{
		"read_count":     c.stats.ReadCount.Load(),
		"write_count":    c.stats.WriteCount.Load(),
		"error_count":    c.stats.ErrorCount.Load(),
		"retry_count":    c.stats.RetryCount.Load(),
		"total_read_ns":  uint64(c.stats.TotalReadTime.Load()),
		"total_write_ns": uint64(c.stats.TotalWriteTime.Load()),
	}
}

// GetStatsStruct returns the raw stats struct for direct access.
// Deprecated: Use GetDeviceStats for structured stats.
func (c *Client) GetStatsStruct() *ClientStats {
	return c.stats
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

// ConsecutiveFailures returns the current consecutive failure count.
func (c *Client) ConsecutiveFailures() int32 {
	return c.consecutiveFailures.Load()
}

// GetAllDeviceHealth returns health info for all devices in the pool.
func (p *ConnectionPool) GetAllDeviceHealth() map[string]DeviceHealth {
	p.mu.RLock()
	defer p.mu.RUnlock()

	result := make(map[string]DeviceHealth)
	for deviceID, pc := range p.clients {
		pc.mu.Lock()
		health := DeviceHealth{
			DeviceID:           deviceID,
			Connected:          pc.client.IsConnected(),
			CircuitBreakerOpen: pc.breaker.State().String() == "open",
			LastError:          pc.lastError,
		}
		pc.mu.Unlock()
		result[deviceID] = health
	}
	return result
}

// GetPoolStats returns comprehensive pool statistics.
func (p *ConnectionPool) GetPoolStats() PoolStats {
	return p.Stats()
}

// IsHealthy returns true if the pool is healthy and accepting connections.
func (p *ConnectionPool) IsHealthy() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return !p.closed
}
