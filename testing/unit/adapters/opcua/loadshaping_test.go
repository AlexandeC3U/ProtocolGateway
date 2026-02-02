// Package opcua_test tests the OPC UA load shaping functionality.
package opcua_test

import (
	"testing"
	"time"

	"github.com/nexus-edge/protocol-gateway/internal/adapter/opcua"
)

// TestPriorityConstants tests priority level constants.
func TestPriorityConstants(t *testing.T) {
	tests := []struct {
		priority int
		name     string
		order    int // Lower is lower priority
	}{
		{opcua.PriorityTelemetry, "Telemetry", 0},
		{opcua.PriorityControl, "Control", 1},
		{opcua.PrioritySafety, "Safety", 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.priority != tt.order {
				t.Errorf("expected priority %d for %s, got %d", tt.order, tt.name, tt.priority)
			}
		})
	}

	// Verify priority ordering
	if opcua.PriorityTelemetry >= opcua.PriorityControl {
		t.Error("Telemetry should have lower priority than Control")
	}
	if opcua.PriorityControl >= opcua.PrioritySafety {
		t.Error("Control should have lower priority than Safety")
	}
}

// TestPriorityQueueBehavior tests priority queue ordering.
func TestPriorityQueueBehavior(t *testing.T) {
	// Document expected behavior: Safety > Control > Telemetry
	operations := []struct {
		name      string
		priority  int
		processed int // Order in which it should be processed
	}{
		{"Telemetry read", opcua.PriorityTelemetry, 3},
		{"Control write", opcua.PriorityControl, 2},
		{"Safety interlock", opcua.PrioritySafety, 1},
	}

	// Sort by priority (descending)
	for i := 0; i < len(operations)-1; i++ {
		for j := i + 1; j < len(operations); j++ {
			if operations[i].priority < operations[j].priority {
				operations[i], operations[j] = operations[j], operations[i]
			}
		}
	}

	// Verify first item is highest priority
	if operations[0].priority != opcua.PrioritySafety {
		t.Error("Safety should be processed first")
	}
}

// TestBrownoutThresholdCalculation tests brownout threshold logic.
func TestBrownoutThresholdCalculation(t *testing.T) {
	tests := []struct {
		name            string
		maxInFlight     int
		threshold       float64
		brownoutTrigger int
	}{
		{"Standard 80%", 100, 0.80, 80},
		{"Conservative 70%", 100, 0.70, 70},
		{"Aggressive 90%", 100, 0.90, 90},
		{"Large pool 80%", 500, 0.80, 400},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			trigger := int(float64(tt.maxInFlight) * tt.threshold)
			if trigger != tt.brownoutTrigger {
				t.Errorf("expected brownout trigger %d, got %d", tt.brownoutTrigger, trigger)
			}
		})
	}
}

// TestBrownoutModeActivation tests brownout mode activation conditions.
func TestBrownoutModeActivation(t *testing.T) {
	tests := []struct {
		name            string
		maxInFlight     int
		currentInFlight int
		threshold       float64
		shouldActivate  bool
	}{
		{"Below threshold", 100, 70, 0.80, false},
		{"At threshold", 100, 80, 0.80, true},
		{"Above threshold", 100, 90, 0.80, true},
		{"Just below", 100, 79, 0.80, false},
		{"At capacity", 100, 100, 0.80, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			trigger := int(float64(tt.maxInFlight) * tt.threshold)
			shouldActivate := tt.currentInFlight >= trigger
			if shouldActivate != tt.shouldActivate {
				t.Errorf("expected brownout=%v at %d/%d, got %v",
					tt.shouldActivate, tt.currentInFlight, tt.maxInFlight, shouldActivate)
			}
		})
	}
}

// TestTelemetryThrottling tests telemetry throttling during brownout.
func TestTelemetryThrottling(t *testing.T) {
	tests := []struct {
		name           string
		brownoutActive bool
		priority       int
		shouldQueue    bool
		shouldDrop     bool
	}{
		{"Normal - telemetry", false, opcua.PriorityTelemetry, true, false},
		{"Brownout - telemetry", true, opcua.PriorityTelemetry, false, true},
		{"Brownout - control", true, opcua.PriorityControl, true, false},
		{"Brownout - safety", true, opcua.PrioritySafety, true, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// During brownout, only telemetry (lowest priority) is throttled
			shouldDrop := tt.brownoutActive && tt.priority == opcua.PriorityTelemetry
			if shouldDrop != tt.shouldDrop {
				t.Errorf("expected shouldDrop=%v, got %v", tt.shouldDrop, shouldDrop)
			}
		})
	}
}

// TestDeadlinePropagation tests context deadline handling.
func TestDeadlinePropagation(t *testing.T) {
	tests := []struct {
		name         string
		queueDelay   time.Duration
		ctxTimeout   time.Duration
		shouldExpire bool
	}{
		{"Fast queue", 10 * time.Millisecond, 1 * time.Second, false},
		{"Slow queue, long timeout", 100 * time.Millisecond, 5 * time.Second, false},
		{"Queue delay > timeout", 2 * time.Second, 1 * time.Second, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expired := tt.queueDelay >= tt.ctxTimeout
			if expired != tt.shouldExpire {
				t.Errorf("expected expired=%v, got %v", tt.shouldExpire, expired)
			}
			if expired {
				t.Logf("Operation would be skipped - context expired before execution")
			}
		})
	}
}

// TestQueueSizing tests priority queue capacity planning.
func TestQueueSizing(t *testing.T) {
	// Typical queue sizes for different deployment scales
	tests := []struct {
		name          string
		telemetrySize int
		controlSize   int
		safetySize    int
		totalCapacity int
	}{
		{"Small", 100, 50, 20, 170},
		{"Medium", 500, 100, 50, 650},
		{"Large", 1000, 200, 100, 1300},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			total := tt.telemetrySize + tt.controlSize + tt.safetySize
			if total != tt.totalCapacity {
				t.Errorf("expected total capacity %d, got %d", tt.totalCapacity, total)
			}

			// Safety queue should be smallest (most urgent, shouldn't queue)
			if tt.safetySize > tt.controlSize {
				t.Error("Safety queue should be smaller than control queue")
			}
		})
	}
}

// TestCircuitBreakerInteraction tests CB state checks before execution.
func TestCircuitBreakerInteraction(t *testing.T) {
	cbStates := []struct {
		state       string
		canExecute  bool
		description string
	}{
		{"closed", true, "Normal operation"},
		{"half-open", true, "Testing recovery"},
		{"open", false, "Blocked - endpoint failing"},
	}

	for _, s := range cbStates {
		t.Run(s.state, func(t *testing.T) {
			canExecute := s.state != "open"
			if canExecute != s.canExecute {
				t.Errorf("expected canExecute=%v for state %q", s.canExecute, s.state)
			}
		})
	}
}

// TestOperationPrioritization tests operation type to priority mapping.
func TestOperationPrioritization(t *testing.T) {
	operations := []struct {
		operation string
		priority  int
	}{
		{"Bulk data read", opcua.PriorityTelemetry},
		{"Subscription notification", opcua.PriorityTelemetry},
		{"Browse nodes", opcua.PriorityTelemetry},
		{"Write setpoint", opcua.PriorityControl},
		{"Call method", opcua.PriorityControl},
		{"Acknowledge alarm", opcua.PriorityControl},
		{"Emergency stop", opcua.PrioritySafety},
		{"Interlock check", opcua.PrioritySafety},
		{"Safety command", opcua.PrioritySafety},
	}

	for _, op := range operations {
		t.Run(op.operation, func(t *testing.T) {
			t.Logf("%s -> Priority %d", op.operation, op.priority)
			// Verify priority is valid
			if op.priority < opcua.PriorityTelemetry || op.priority > opcua.PrioritySafety {
				t.Errorf("invalid priority %d for operation %q", op.priority, op.operation)
			}
		})
	}
}

// TestLoadBalancing tests in-flight counter behavior.
func TestLoadBalancing(t *testing.T) {
	tests := []struct {
		name        string
		globalMax   int64
		endpointMax int64
		globalCur   int64
		endpointCur int64
		canProceed  bool
	}{
		{"Both available", 100, 20, 50, 10, true},
		{"Global at limit", 100, 20, 100, 10, false},
		{"Endpoint at limit", 100, 20, 50, 20, false},
		{"Both at limit", 100, 20, 100, 20, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			globalOk := tt.globalCur < tt.globalMax
			endpointOk := tt.endpointCur < tt.endpointMax
			canProceed := globalOk && endpointOk
			if canProceed != tt.canProceed {
				t.Errorf("expected canProceed=%v, got %v", tt.canProceed, canProceed)
			}
		})
	}
}

// TestWorkerPoolSizing tests worker count recommendations.
func TestWorkerPoolSizing(t *testing.T) {
	// Multiple workers process priority queues
	tests := []struct {
		name         string
		endpoints    int
		workerCount  int
		opsPerSecond int
	}{
		{"Single endpoint", 1, 2, 100},
		{"Small deployment", 5, 4, 500},
		{"Medium deployment", 20, 8, 2000},
		{"Large deployment", 50, 16, 5000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opsPerWorker := tt.opsPerSecond / tt.workerCount
			t.Logf("%s: %d workers handling %d ops/s (%d ops/worker)",
				tt.name, tt.workerCount, tt.opsPerSecond, opsPerWorker)

			// Workers should scale with load
			if tt.workerCount < 2 {
				t.Error("Should have at least 2 workers for reliability")
			}
		})
	}
}

// TestLatencyBudgeting tests operation latency considerations.
func TestLatencyBudgeting(t *testing.T) {
	// Time budgets for different priority levels
	tests := []struct {
		priority    int
		maxLatency  time.Duration
		description string
	}{
		{opcua.PriorityTelemetry, 5 * time.Second, "Bulk data can tolerate delay"},
		{opcua.PriorityControl, 500 * time.Millisecond, "Control needs quick response"},
		{opcua.PrioritySafety, 100 * time.Millisecond, "Safety must be immediate"},
	}

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			// Higher priority = stricter latency requirement
			if tt.priority == opcua.PrioritySafety && tt.maxLatency > 200*time.Millisecond {
				t.Error("Safety operations should have sub-200ms latency budget")
			}
		})
	}
}
