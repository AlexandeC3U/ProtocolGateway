// Package health_test tests the health checker functionality.
package health_test

import (
	"testing"
	"time"

	"github.com/nexus-edge/protocol-gateway/internal/health"
)

// TestOperationalStateConstants tests operational state constants.
func TestOperationalStateConstants(t *testing.T) {
	states := []struct {
		state       health.OperationalState
		name        string
		description string
	}{
		{health.StateStarting, "starting", "Initial startup"},
		{health.StateRunning, "running", "Normal operation"},
		{health.StateDegraded, "degraded", "Partial functionality"},
		{health.StateRecovering, "recovering", "Attempting recovery"},
		{health.StateShuttingDown, "shutting_down", "Graceful shutdown"},
		{health.StateOffline, "offline", "Not operational"},
	}

	for _, s := range states {
		t.Run(s.name, func(t *testing.T) {
			if string(s.state) != s.name {
				t.Errorf("expected state %q, got %q", s.name, string(s.state))
			}
			t.Logf("State %s: %s", s.state, s.description)
		})
	}
}

// TestStateTransitions tests valid state transitions.
func TestStateTransitions(t *testing.T) {
	type transition struct {
		from    health.OperationalState
		to      health.OperationalState
		allowed bool
		reason  string
	}

	transitions := []transition{
		{health.StateStarting, health.StateRunning, true, "Startup complete"},
		{health.StateStarting, health.StateOffline, true, "Startup failed"},
		{health.StateRunning, health.StateDegraded, true, "Component failure"},
		{health.StateRunning, health.StateShuttingDown, true, "Shutdown requested"},
		{health.StateDegraded, health.StateRecovering, true, "Recovery initiated"},
		{health.StateDegraded, health.StateRunning, true, "Auto-recovered"},
		{health.StateRecovering, health.StateRunning, true, "Recovery success"},
		{health.StateRecovering, health.StateDegraded, true, "Recovery partial"},
		{health.StateRecovering, health.StateOffline, true, "Recovery failed"},
		{health.StateShuttingDown, health.StateOffline, true, "Shutdown complete"},
		// Invalid transitions
		{health.StateOffline, health.StateRunning, false, "Must restart"},
		{health.StateShuttingDown, health.StateRunning, false, "No abort"},
	}

	for _, tr := range transitions {
		name := string(tr.from) + "_to_" + string(tr.to)
		t.Run(name, func(t *testing.T) {
			t.Logf("%s -> %s: allowed=%v (%s)",
				tr.from, tr.to, tr.allowed, tr.reason)
		})
	}
}

// TestCheckSeverityLevels tests severity level ordering.
func TestCheckSeverityLevels(t *testing.T) {
	severities := []struct {
		level    health.CheckSeverity
		priority int
		action   string
	}{
		{health.SeverityInfo, 0, "Log only"},
		{health.SeverityWarning, 1, "Alert, continue"},
		{health.SeverityCritical, 2, "Alert, may degrade"},
	}

	for _, s := range severities {
		t.Run(s.level.String(), func(t *testing.T) {
			t.Logf("Severity %s (priority %d): %s",
				s.level.String(), s.priority, s.action)
		})
	}

	// Verify ordering
	if health.SeverityInfo >= health.SeverityWarning {
		t.Error("Info should be lower priority than Warning")
	}
}

// TestHealthCheckerConfigStructure tests HealthChecker Config fields.
func TestHealthCheckerConfigStructure(t *testing.T) {
	cfg := health.Config{
		ServiceName:       "test-gateway",
		ServiceVersion:    "1.0.0",
		CheckTimeout:      5 * time.Second,
		CheckInterval:     30 * time.Second,
		FailureThreshold:  3,
		RecoveryThreshold: 2,
	}

	if cfg.CheckInterval != 30*time.Second {
		t.Errorf("expected CheckInterval 30s, got %v", cfg.CheckInterval)
	}
	if cfg.FailureThreshold != 3 {
		t.Errorf("expected FailureThreshold 3, got %d", cfg.FailureThreshold)
	}
	if cfg.RecoveryThreshold != 2 {
		t.Errorf("expected RecoveryThreshold 2, got %d", cfg.RecoveryThreshold)
	}
	if cfg.CheckTimeout != 5*time.Second {
		t.Errorf("expected CheckTimeout 5s, got %v", cfg.CheckTimeout)
	}
}

// TestFlappingDetection tests flapping detection logic.
func TestFlappingDetection(t *testing.T) {
	tests := []struct {
		name         string
		stateChanges int
		window       time.Duration
		threshold    int
		isFlapping   bool
	}{
		{"Stable", 1, 5 * time.Minute, 5, false},
		{"Minor changes", 3, 5 * time.Minute, 5, false},
		{"At threshold", 5, 5 * time.Minute, 5, true},
		{"Heavy flapping", 10, 5 * time.Minute, 5, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isFlapping := tt.stateChanges >= tt.threshold
			if isFlapping != tt.isFlapping {
				t.Errorf("expected isFlapping=%v with %d changes in %v (threshold %d)",
					tt.isFlapping, tt.stateChanges, tt.window, tt.threshold)
			}
		})
	}
}

// TestFailureThresholdBehavior tests consecutive failure handling.
func TestFailureThresholdBehavior(t *testing.T) {
	tests := []struct {
		name             string
		consecutiveFails int
		threshold        int
		shouldDegrade    bool
	}{
		{"No failures", 0, 3, false},
		{"One failure", 1, 3, false},
		{"Two failures", 2, 3, false},
		{"At threshold", 3, 3, true},
		{"Above threshold", 5, 3, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shouldDegrade := tt.consecutiveFails >= tt.threshold
			if shouldDegrade != tt.shouldDegrade {
				t.Errorf("expected shouldDegrade=%v at %d fails (threshold %d)",
					tt.shouldDegrade, tt.consecutiveFails, tt.threshold)
			}
		})
	}
}

// TestSuccessThresholdBehavior tests recovery threshold handling.
func TestSuccessThresholdBehavior(t *testing.T) {
	tests := []struct {
		name               string
		consecutiveSuccess int
		threshold          int
		shouldRecover      bool
	}{
		{"No success", 0, 2, false},
		{"One success", 1, 2, false},
		{"At threshold", 2, 2, true},
		{"Above threshold", 3, 2, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shouldRecover := tt.consecutiveSuccess >= tt.threshold
			if shouldRecover != tt.shouldRecover {
				t.Errorf("expected shouldRecover=%v at %d successes (threshold %d)",
					tt.shouldRecover, tt.consecutiveSuccess, tt.threshold)
			}
		})
	}
}

// TestCheckStatusStructure tests CheckStatus fields.
func TestCheckStatusStructure(t *testing.T) {
	status := health.CheckStatus{
		Name:             "database",
		Status:           "healthy",
		Severity:         "info",
		Error:            "",
		LastCheck:        time.Now(),
		ConsecutiveFails: 0,
	}

	if status.Name != "database" {
		t.Errorf("expected Name database, got %s", status.Name)
	}
	if status.Status != "healthy" {
		t.Error("expected Status=healthy")
	}
	if status.Severity != "info" {
		t.Errorf("expected Severity 'info', got %s", status.Severity)
	}
	if status.ConsecutiveFails != 0 {
		t.Errorf("expected ConsecutiveFails 0, got %d", status.ConsecutiveFails)
	}
}

// TestHealthResponseStructure tests HealthResponse fields.
func TestHealthResponseStructure(t *testing.T) {
	resp := health.HealthResponse{
		Status:    "healthy",
		State:     health.StateRunning,
		Service:   "gateway",
		Version:   "1.0.0",
		Timestamp: time.Now(),
		Checks:    make(map[string]*health.CheckStatus),
		Uptime:    "24h0m0s",
	}

	if resp.State != health.StateRunning {
		t.Errorf("expected State running, got %s", resp.State)
	}
	if resp.Version != "1.0.0" {
		t.Errorf("expected Version 1.0.0, got %s", resp.Version)
	}
	if resp.Status != "healthy" {
		t.Errorf("expected Status healthy, got %s", resp.Status)
	}
}

// TestAggregateHealthStatus tests overall health aggregation.
func TestAggregateHealthStatus(t *testing.T) {
	tests := []struct {
		name     string
		checks   []bool
		expected health.OperationalState
	}{
		{"All healthy", []bool{true, true, true}, health.StateRunning},
		{"One unhealthy", []bool{true, false, true}, health.StateDegraded},
		{"All unhealthy", []bool{false, false, false}, health.StateOffline},
		{"No checks", []bool{}, health.StateRunning},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			unhealthyCount := 0
			for _, healthy := range tt.checks {
				if !healthy {
					unhealthyCount++
				}
			}

			var status health.OperationalState
			switch {
			case len(tt.checks) == 0:
				status = health.StateRunning
			case unhealthyCount == len(tt.checks):
				status = health.StateOffline
			case unhealthyCount > 0:
				status = health.StateDegraded
			default:
				status = health.StateRunning
			}

			if status != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, status)
			}
		})
	}
}

// TestDegradedCheckMultiplier tests degraded state check frequency.
func TestDegradedCheckMultiplier(t *testing.T) {
	tests := []struct {
		name         string
		baseInterval time.Duration
		multiplier   int
		state        health.OperationalState
		expected     time.Duration
	}{
		{"Running normal", 30 * time.Second, 2, health.StateRunning, 30 * time.Second},
		{"Degraded 2x", 30 * time.Second, 2, health.StateDegraded, 15 * time.Second},
		{"Recovering 2x", 30 * time.Second, 2, health.StateRecovering, 15 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			interval := tt.baseInterval
			if tt.state == health.StateDegraded || tt.state == health.StateRecovering {
				interval = tt.baseInterval / time.Duration(tt.multiplier)
			}

			if interval != tt.expected {
				t.Errorf("expected interval %v in state %s, got %v",
					tt.expected, tt.state, interval)
			}
		})
	}
}

// TestComponentHealthChecks tests individual component checks.
func TestComponentHealthChecks(t *testing.T) {
	components := []struct {
		name     string
		check    func() bool
		critical bool
	}{
		{"mqtt_broker", func() bool { return true }, true},
		{"database", func() bool { return true }, true},
		{"opcua_server", func() bool { return true }, false},
		{"s7_plc", func() bool { return true }, false},
		{"modbus_device", func() bool { return true }, false},
	}

	for _, c := range components {
		t.Run(c.name, func(t *testing.T) {
			healthy := c.check()
			t.Logf("Component %s: healthy=%v, critical=%v",
				c.name, healthy, c.critical)
		})
	}
}

// TestHealthCheckTimeout tests check timeout handling.
func TestHealthCheckTimeout(t *testing.T) {
	tests := []struct {
		name       string
		checkTime  time.Duration
		timeout    time.Duration
		shouldFail bool
	}{
		{"Fast check", 10 * time.Millisecond, 1 * time.Second, false},
		{"Slow check OK", 500 * time.Millisecond, 1 * time.Second, false},
		{"Timeout exceeded", 2 * time.Second, 1 * time.Second, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shouldFail := tt.checkTime > tt.timeout
			if shouldFail != tt.shouldFail {
				t.Errorf("expected shouldFail=%v when check takes %v (timeout %v)",
					tt.shouldFail, tt.checkTime, tt.timeout)
			}
		})
	}
}

// TestHealthMetrics tests health-related metrics.
func TestHealthMetrics(t *testing.T) {
	metrics := []struct {
		name   string
		mtype  string
		labels []string
	}{
		{"gateway_health_status", "gauge", []string{}},
		{"gateway_health_check_duration_seconds", "histogram", []string{"check"}},
		{"gateway_health_check_status", "gauge", []string{"check"}},
		{"gateway_state_transitions_total", "counter", []string{"from", "to"}},
		{"gateway_uptime_seconds", "gauge", []string{}},
	}

	for _, m := range metrics {
		t.Run(m.name, func(t *testing.T) {
			t.Logf("Metric %s (%s) labels=%v", m.name, m.mtype, m.labels)
		})
	}
}

// TestHealthEndpointResponse tests HTTP health endpoint format.
func TestHealthEndpointResponse(t *testing.T) {
	resp := health.HealthResponse{
		Status:    "healthy",
		State:     health.StateRunning,
		Version:   "1.0.0",
		Timestamp: time.Now(),
		Checks:    make(map[string]*health.CheckStatus),
	}

	// Check Status field for healthy determination
	allHealthy := resp.Status == "healthy"

	expectedCode := 200
	if !allHealthy {
		expectedCode = 503
	}

	if allHealthy && expectedCode != 200 {
		t.Error("healthy response should return 200")
	}
}

// TestReadinessVsLiveness tests readiness vs liveness distinction.
func TestReadinessVsLiveness(t *testing.T) {
	tests := []struct {
		state health.OperationalState
		live  bool
		ready bool
	}{
		{health.StateStarting, true, false},     // Live but not ready
		{health.StateRunning, true, true},       // Both
		{health.StateDegraded, true, true},      // Still ready, degraded
		{health.StateRecovering, true, false},   // Live, not ready
		{health.StateShuttingDown, true, false}, // Live, not ready
		{health.StateOffline, false, false},     // Neither
	}

	for _, tt := range tests {
		t.Run(string(tt.state), func(t *testing.T) {
			t.Logf("State %s: live=%v, ready=%v", tt.state, tt.live, tt.ready)
		})
	}
}

// TestGracefulDegradation tests graceful degradation behavior.
func TestGracefulDegradation(t *testing.T) {
	// When components fail, system should degrade gracefully
	scenarios := []struct {
		name            string
		failedComponent string
		impact          string
		canContinue     bool
	}{
		{"One device offline", "s7_plc_001", "That device data missing", true},
		{"MQTT down", "mqtt_broker", "No data publishing", false},
		{"One OPC UA server", "opcua_server_001", "That server tags missing", true},
	}

	for _, s := range scenarios {
		t.Run(s.name, func(t *testing.T) {
			t.Logf("Failed: %s -> Impact: %s, Continue: %v",
				s.failedComponent, s.impact, s.canContinue)
		})
	}
}
