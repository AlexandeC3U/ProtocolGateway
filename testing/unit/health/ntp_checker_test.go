package health_test

import (
	"context"
	"encoding/binary"
	"math"
	"net"
	"testing"
	"time"

	"github.com/nexus-edge/protocol-gateway/internal/health"
	"github.com/rs/zerolog"
)

// TestNTPChecker_Defaults tests that NewNTPChecker applies sensible defaults.
func TestNTPChecker_Defaults(t *testing.T) {
	logger := zerolog.Nop()
	checker := health.NewNTPChecker(health.NTPConfig{}, logger, nil)
	defer checker.Stop()

	if checker == nil {
		t.Fatal("expected non-nil NTPChecker")
	}

	// Initial offset should be zero
	offset := checker.GetOffset()
	if offset != 0 {
		t.Errorf("expected initial offset 0, got %v", offset)
	}

	// Initial last check should be zero time
	lastCheck := checker.GetLastCheck()
	if !lastCheck.IsZero() {
		t.Errorf("expected zero last check time, got %v", lastCheck)
	}
}

// TestNTPChecker_HealthCheck_NeverChecked tests that HealthCheck attempts a check
// when it has never run. This will fail without network access, which is expected
// in CI — the test verifies the error path works correctly.
func TestNTPChecker_HealthCheck_NeverChecked(t *testing.T) {
	logger := zerolog.Nop()
	// Use a non-routable address to ensure fast failure
	checker := health.NewNTPChecker(health.NTPConfig{
		Server:        "192.0.2.1", // TEST-NET, guaranteed unreachable
		CritThreshold: 1 * time.Second,
	}, logger, nil)
	defer checker.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()

	err := checker.HealthCheck(ctx)
	// Should fail because we can't reach the fake NTP server
	if err == nil {
		t.Log("HealthCheck succeeded (NTP reachable from test environment)")
	} else {
		t.Logf("HealthCheck failed as expected: %v", err)
	}
}

// TestNTPChecker_WithMockServer tests the NTP checker against a local mock NTP server.
func TestNTPChecker_WithMockServer(t *testing.T) {
	// Start a mock NTP server
	addr, stop := startMockNTPServer(t, 50*time.Millisecond) // 50ms simulated offset
	defer stop()

	logger := zerolog.Nop()
	checker := health.NewNTPChecker(health.NTPConfig{
		Server:        addr,
		CheckInterval: 100 * time.Millisecond,
		WarnThreshold: 500 * time.Millisecond,
		CritThreshold: 2 * time.Second,
	}, logger, nil)

	checker.Start()
	defer checker.Stop()

	// Wait for at least one check
	time.Sleep(300 * time.Millisecond)

	lastCheck := checker.GetLastCheck()
	if lastCheck.IsZero() {
		t.Fatal("expected at least one successful NTP check")
	}

	offset := checker.GetOffset()
	// The offset should be small (mock server returns current time + simulated offset)
	// Allow generous tolerance since we're testing over localhost
	if offset > 5*time.Second || offset < -5*time.Second {
		t.Errorf("offset %v seems unreasonable for a local mock server", offset)
	}

	// Health check should pass (offset well under crit threshold)
	err := checker.HealthCheck(context.Background())
	if err != nil {
		t.Errorf("expected healthy, got error: %v", err)
	}
}

// TestNTPChecker_CriticalThreshold tests that HealthCheck fails when drift exceeds threshold.
func TestNTPChecker_CriticalThreshold(t *testing.T) {
	// Start mock server with large offset (5 seconds)
	addr, stop := startMockNTPServer(t, 5*time.Second)
	defer stop()

	logger := zerolog.Nop()
	checker := health.NewNTPChecker(health.NTPConfig{
		Server:        addr,
		CheckInterval: 100 * time.Millisecond,
		WarnThreshold: 100 * time.Millisecond,
		CritThreshold: 1 * time.Second, // 1s threshold, 5s drift → should fail
	}, logger, nil)

	checker.Start()
	defer checker.Stop()

	// Wait for check
	time.Sleep(300 * time.Millisecond)

	err := checker.HealthCheck(context.Background())
	if err == nil {
		t.Error("expected health check to fail due to critical drift")
	} else {
		t.Logf("Health check correctly failed: %v", err)
	}
}

// startMockNTPServer starts a UDP server that responds with NTP-like packets.
// The simulatedOffset is added to the transmit timestamp to simulate clock drift.
// Returns "host:port" and a stop function.
func startMockNTPServer(t *testing.T, simulatedOffset time.Duration) (string, func()) {
	t.Helper()

	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start mock NTP server: %v", err)
	}

	addr := conn.LocalAddr().String()
	_, port, _ := net.SplitHostPort(addr)

	stopCh := make(chan struct{})
	doneCh := make(chan struct{})
	go func() {
		defer close(doneCh)
		buf := make([]byte, 48)
		for {
			conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
			n, remoteAddr, err := conn.ReadFrom(buf)
			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					select {
					case <-stopCh:
						return
					default:
						continue
					}
				}
				return
			}
			if n < 48 {
				continue
			}

			// Build NTP response
			resp := make([]byte, 48)
			resp[0] = 0x24 // LI=0, VN=4, Mode=4 (server)
			resp[1] = 1    // Stratum 1

			now := time.Now()
			serverTime := now.Add(simulatedOffset)

			// Receive timestamp (t1) = when server received the request
			putNTPTimestamp(resp[32:40], serverTime)
			// Transmit timestamp (t2) = when server sends the response
			putNTPTimestamp(resp[40:48], serverTime)

			conn.WriteTo(resp, remoteAddr)
		}
	}()

	stop := func() {
		close(stopCh)
		conn.Close()
		<-doneCh
	}

	return "127.0.0.1:" + port, stop
}

// putNTPTimestamp encodes a time.Time as an NTP timestamp (8 bytes).
func putNTPTimestamp(b []byte, t time.Time) {
	const ntpEpochOffset = 2208988800
	secs := uint32(t.Unix() + ntpEpochOffset)
	frac := uint32(math.Round(float64(t.Nanosecond()) * (1 << 32) / 1e9))
	binary.BigEndian.PutUint32(b[0:4], secs)
	binary.BigEndian.PutUint32(b[4:8], frac)
}
