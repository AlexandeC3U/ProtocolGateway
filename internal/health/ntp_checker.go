// Package health provides health check functionality for the service.
package health

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nexus-edge/protocol-gateway/internal/metrics"
	"github.com/rs/zerolog"
)

// NTPConfig holds NTP checker configuration.
type NTPConfig struct {
	Enabled       bool
	Server        string
	CheckInterval time.Duration
	WarnThreshold time.Duration
	CritThreshold time.Duration
}

// NTPChecker monitors clock drift by querying an NTP server periodically.
// It implements the Checker interface for integration with the health system.
type NTPChecker struct {
	config  NTPConfig
	metrics *metrics.Registry
	logger  zerolog.Logger

	// Last measured offset (atomic for thread-safe reads)
	lastOffset atomic.Value // stores time.Duration
	lastCheck  atomic.Value // stores time.Time

	// Last error (mutex-protected since atomic.Value can't store nil)
	lastErr   error
	lastErrMu sync.RWMutex

	stopChan chan struct{}
	stopOnce sync.Once
	wg       sync.WaitGroup
}

// NewNTPChecker creates a new NTP clock drift checker.
func NewNTPChecker(config NTPConfig, logger zerolog.Logger, metricsReg *metrics.Registry) *NTPChecker {
	if config.Server == "" {
		config.Server = "pool.ntp.org"
	}
	if config.CheckInterval <= 0 {
		config.CheckInterval = 5 * time.Minute
	}
	if config.WarnThreshold <= 0 {
		config.WarnThreshold = 500 * time.Millisecond
	}
	if config.CritThreshold <= 0 {
		config.CritThreshold = 2 * time.Second
	}

	checker := &NTPChecker{
		config:   config,
		metrics:  metricsReg,
		logger:   logger.With().Str("component", "ntp-checker").Logger(),
		stopChan: make(chan struct{}),
	}

	// Initialize atomic values
	checker.lastOffset.Store(time.Duration(0))
	checker.lastCheck.Store(time.Time{})

	return checker
}

// Start begins the background NTP check loop.
func (n *NTPChecker) Start() {
	n.wg.Add(1)
	go n.checkLoop()
	n.logger.Info().
		Str("server", n.config.Server).
		Dur("interval", n.config.CheckInterval).
		Dur("warn_threshold", n.config.WarnThreshold).
		Dur("crit_threshold", n.config.CritThreshold).
		Msg("NTP clock drift checker started")
}

// Stop stops the background NTP check loop.
func (n *NTPChecker) Stop() {
	n.stopOnce.Do(func() {
		close(n.stopChan)
	})
	n.wg.Wait()
}

// HealthCheck implements the Checker interface.
// Returns an error if the clock drift exceeds the critical threshold.
func (n *NTPChecker) HealthCheck(ctx context.Context) error {
	// If we've never successfully checked, try now
	lastCheck := n.lastCheck.Load().(time.Time)
	if lastCheck.IsZero() {
		offset, err := n.queryNTP()
		if err != nil {
			return fmt.Errorf("ntp check failed: %w", err)
		}
		n.recordResult(offset, nil)
	}

	// Check last stored error
	n.lastErrMu.RLock()
	lastErr := n.lastErr
	n.lastErrMu.RUnlock()
	if lastErr != nil {
		return fmt.Errorf("last ntp check failed: %v", lastErr)
	}

	offset := n.lastOffset.Load().(time.Duration)
	absOffset := offset
	if absOffset < 0 {
		absOffset = -absOffset
	}

	if absOffset > n.config.CritThreshold {
		return fmt.Errorf("clock drift %.1fms exceeds critical threshold %.1fms",
			float64(offset.Milliseconds()), float64(n.config.CritThreshold.Milliseconds()))
	}

	return nil
}

// GetOffset returns the last measured NTP offset.
// Positive means gateway clock is ahead of NTP, negative means behind.
func (n *NTPChecker) GetOffset() time.Duration {
	return n.lastOffset.Load().(time.Duration)
}

// GetLastCheck returns the time of the last successful NTP check.
func (n *NTPChecker) GetLastCheck() time.Time {
	return n.lastCheck.Load().(time.Time)
}

func (n *NTPChecker) checkLoop() {
	defer n.wg.Done()

	// Run initial check immediately
	n.runCheck()

	ticker := time.NewTicker(n.config.CheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-n.stopChan:
			return
		case <-ticker.C:
			n.runCheck()
		}
	}
}

func (n *NTPChecker) runCheck() {
	offset, err := n.queryNTP()
	n.recordResult(offset, err)
}

func (n *NTPChecker) recordResult(offset time.Duration, err error) {
	if err != nil {
		n.lastErrMu.Lock()
		n.lastErr = err
		n.lastErrMu.Unlock()
		n.logger.Warn().Err(err).Msg("NTP check failed")
		if n.metrics != nil {
			n.metrics.RecordClockDrift(0, false)
		}
		return
	}

	n.lastOffset.Store(offset)
	n.lastCheck.Store(time.Now())
	n.lastErrMu.Lock()
	n.lastErr = nil
	n.lastErrMu.Unlock()

	offsetSeconds := offset.Seconds()
	absOffset := offset
	if absOffset < 0 {
		absOffset = -absOffset
	}

	if n.metrics != nil {
		n.metrics.RecordClockDrift(offsetSeconds, true)
	}

	if absOffset > n.config.WarnThreshold {
		n.logger.Warn().
			Float64("drift_ms", float64(offset.Milliseconds())).
			Str("server", n.config.Server).
			Msg("Clock drift exceeds warning threshold")
	} else {
		n.logger.Debug().
			Float64("drift_ms", float64(offset.Milliseconds())).
			Msg("NTP clock check OK")
	}
}

// queryNTP performs a single SNTP query and returns the clock offset.
// Uses the standard NTPv4 packet format (RFC 5905) over UDP.
// The offset is calculated as: ((t1-t0) + (t2-t3)) / 2
// where t0=client send, t1=server receive, t2=server transmit, t3=client receive.
func (n *NTPChecker) queryNTP() (time.Duration, error) {
	// If the server already includes a port (e.g., "127.0.0.1:12345"), use it as-is.
	// Otherwise, default to NTP port 123.
	addr := n.config.Server
	if _, _, err := net.SplitHostPort(addr); err != nil {
		addr = net.JoinHostPort(addr, "123")
	}

	conn, err := net.DialTimeout("udp", addr, 5*time.Second)
	if err != nil {
		return 0, fmt.Errorf("dial ntp server %s: %w", n.config.Server, err)
	}
	defer conn.Close()

	if err := conn.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return 0, fmt.Errorf("set deadline: %w", err)
	}

	// Build NTP request packet (48 bytes)
	// LI=0, Version=4, Mode=3 (client) → first byte = 0x23
	req := make([]byte, 48)
	req[0] = 0x23 // LI=0, VN=4, Mode=3

	t0 := time.Now()

	if _, err := conn.Write(req); err != nil {
		return 0, fmt.Errorf("send ntp request: %w", err)
	}

	resp := make([]byte, 48)
	if _, err := conn.Read(resp); err != nil {
		return 0, fmt.Errorf("read ntp response: %w", err)
	}

	t3 := time.Now()

	// Parse server timestamps from response
	// Receive Timestamp (t1): bytes 32-39
	// Transmit Timestamp (t2): bytes 40-47
	t1 := ntpTimestampToTime(binary.BigEndian.Uint32(resp[32:36]), binary.BigEndian.Uint32(resp[36:40]))
	t2 := ntpTimestampToTime(binary.BigEndian.Uint32(resp[40:44]), binary.BigEndian.Uint32(resp[44:48]))

	// Validate response — transmit timestamp must not be zero
	if t2.IsZero() || t2.Year() < 2000 {
		return 0, fmt.Errorf("invalid ntp response: zero transmit timestamp")
	}

	// Calculate offset: ((t1-t0) + (t2-t3)) / 2
	offset := (t1.Sub(t0) + t2.Sub(t3)) / 2

	return offset, nil
}

// ntpTimestampToTime converts an NTP timestamp (seconds since 1900-01-01) to time.Time.
// NTP epoch is January 1, 1900; Unix epoch is January 1, 1970.
func ntpTimestampToTime(seconds, fraction uint32) time.Time {
	// Seconds between 1900-01-01 and 1970-01-01
	const ntpEpochOffset = 2208988800

	secs := int64(seconds) - ntpEpochOffset
	// Convert fraction to nanoseconds: fraction * 1e9 / 2^32
	nanos := int64(math.Round(float64(fraction) * 1e9 / (1 << 32)))

	return time.Unix(secs, nanos)
}
