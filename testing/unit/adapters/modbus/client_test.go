// Package modbus_test tests the Modbus client functionality.
package modbus_test

import (
	"testing"
	"time"

	"github.com/nexus-edge/protocol-gateway/internal/adapter/modbus"
	"github.com/nexus-edge/protocol-gateway/internal/domain"
)

// TestClientConfigStructure tests the ClientConfig struct fields.
func TestClientConfigStructure(t *testing.T) {
	cfg := modbus.ClientConfig{
		Address:     "192.168.1.100:502",
		SlaveID:     1,
		Timeout:     5 * time.Second,
		IdleTimeout: 30 * time.Second,
		MaxRetries:  3,
		RetryDelay:  100 * time.Millisecond,
		Protocol:    domain.ProtocolModbusTCP,
	}

	if cfg.Address != "192.168.1.100:502" {
		t.Errorf("expected address '192.168.1.100:502', got %q", cfg.Address)
	}
	if cfg.SlaveID != 1 {
		t.Errorf("expected SlaveID 1, got %d", cfg.SlaveID)
	}
	if cfg.Protocol != domain.ProtocolModbusTCP {
		t.Errorf("expected protocol ModbusTCP, got %v", cfg.Protocol)
	}
}

// TestClientConfigValidSlaveID tests valid slave ID ranges.
func TestClientConfigValidSlaveID(t *testing.T) {
	tests := []struct {
		name    string
		slaveID byte
		valid   bool
	}{
		{"Minimum valid", 1, true},
		{"Middle range", 100, true},
		{"Maximum valid", 247, true},
		{"Zero - broadcast", 0, false},
		{"Above maximum", 248, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isValid := tt.slaveID >= 1 && tt.slaveID <= 247
			if isValid != tt.valid {
				t.Errorf("expected validity %v for slaveID %d, got %v", tt.valid, tt.slaveID, isValid)
			}
		})
	}
}

// TestClientStatsAtomicOperations tests ClientStats atomic operations.
func TestClientStatsAtomicOperations(t *testing.T) {
	stats := &modbus.ClientStats{}

	// Test ReadCount
	stats.ReadCount.Add(5)
	stats.ReadCount.Add(3)
	if got := stats.ReadCount.Load(); got != 8 {
		t.Errorf("expected ReadCount 8, got %d", got)
	}

	// Test WriteCount
	stats.WriteCount.Add(10)
	if got := stats.WriteCount.Load(); got != 10 {
		t.Errorf("expected WriteCount 10, got %d", got)
	}

	// Test ErrorCount
	stats.ErrorCount.Add(2)
	if got := stats.ErrorCount.Load(); got != 2 {
		t.Errorf("expected ErrorCount 2, got %d", got)
	}

	// Test RetryCount
	stats.RetryCount.Add(4)
	if got := stats.RetryCount.Load(); got != 4 {
		t.Errorf("expected RetryCount 4, got %d", got)
	}

	// Test TotalReadTime
	stats.TotalReadTime.Add(1000000) // 1ms in nanoseconds
	if got := stats.TotalReadTime.Load(); got != 1000000 {
		t.Errorf("expected TotalReadTime 1000000, got %d", got)
	}

	// Test TotalWriteTime
	stats.TotalWriteTime.Add(500000)
	if got := stats.TotalWriteTime.Load(); got != 500000 {
		t.Errorf("expected TotalWriteTime 500000, got %d", got)
	}
}

// TestClientStatsConcurrency tests that stats are safe for concurrent access.
func TestClientStatsConcurrency(t *testing.T) {
	stats := &modbus.ClientStats{}
	done := make(chan struct{})

	// Multiple goroutines updating stats
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				stats.ReadCount.Add(1)
				stats.TotalReadTime.Add(1000)
			}
			done <- struct{}{}
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	if got := stats.ReadCount.Load(); got != 1000 {
		t.Errorf("expected ReadCount 1000, got %d", got)
	}
}

// TestTagDiagnosticStructure tests the TagDiagnostic type.
func TestTagDiagnosticStructure(t *testing.T) {
	diag := modbus.NewTagDiagnostic("tag-001")

	if diag.TagID != "tag-001" {
		t.Errorf("expected TagID 'tag-001', got %q", diag.TagID)
	}

	// Test atomic operations
	diag.ReadCount.Add(10)
	diag.ErrorCount.Add(2)

	if got := diag.ReadCount.Load(); got != 10 {
		t.Errorf("expected ReadCount 10, got %d", got)
	}
	if got := diag.ErrorCount.Load(); got != 2 {
		t.Errorf("expected ErrorCount 2, got %d", got)
	}

	// Test storing error
	testErr := domain.ErrConnectionFailed
	diag.LastError.Store(testErr)
	if stored, ok := diag.LastError.Load().(error); ok {
		if stored != testErr {
			t.Errorf("expected stored error %v, got %v", testErr, stored)
		}
	}

	// Test storing time
	now := time.Now()
	diag.LastSuccessTime.Store(now)
	if stored, ok := diag.LastSuccessTime.Load().(time.Time); ok {
		if !stored.Equal(now) {
			t.Errorf("expected stored time %v, got %v", now, stored)
		}
	}
}

// TestRegisterRangeStructure tests the RegisterRange type.
func TestRegisterRangeStructure(t *testing.T) {
	tags := []*domain.Tag{
		{ID: "tag1", Name: "Temperature"},
		{ID: "tag2", Name: "Pressure"},
	}

	rr := modbus.RegisterRange{
		StartAddress: 100,
		EndAddress:   110,
		Tags:         tags,
	}

	if rr.StartAddress != 100 {
		t.Errorf("expected StartAddress 100, got %d", rr.StartAddress)
	}
	if rr.EndAddress != 110 {
		t.Errorf("expected EndAddress 110, got %d", rr.EndAddress)
	}
	if len(rr.Tags) != 2 {
		t.Errorf("expected 2 tags, got %d", len(rr.Tags))
	}
}

// TestBatchConfigDefaults tests DefaultBatchConfig returns proper defaults.
func TestBatchConfigDefaults(t *testing.T) {
	cfg := modbus.DefaultBatchConfig()

	if cfg.MaxRegistersPerRead != 100 {
		t.Errorf("expected MaxRegistersPerRead 100, got %d", cfg.MaxRegistersPerRead)
	}
	if cfg.MaxGapSize != 10 {
		t.Errorf("expected MaxGapSize 10, got %d", cfg.MaxGapSize)
	}
}

// TestBatchConfigCustomValues tests custom BatchConfig values.
func TestBatchConfigCustomValues(t *testing.T) {
	tests := []struct {
		name        string
		maxRegs     uint16
		maxGap      uint16
		description string
	}{
		{"Conservative", 50, 5, "Fewer registers, small gap"},
		{"Aggressive", 125, 20, "Max registers, larger gap"},
		{"Balanced", 100, 10, "Default balanced approach"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := modbus.BatchConfig{
				MaxRegistersPerRead: tt.maxRegs,
				MaxGapSize:          tt.maxGap,
			}
			if cfg.MaxRegistersPerRead != tt.maxRegs {
				t.Errorf("expected MaxRegistersPerRead %d, got %d", tt.maxRegs, cfg.MaxRegistersPerRead)
			}
			if cfg.MaxGapSize != tt.maxGap {
				t.Errorf("expected MaxGapSize %d, got %d", tt.maxGap, cfg.MaxGapSize)
			}
		})
	}
}

// TestPoolConfigDefaults tests DefaultPoolConfig returns proper defaults.
func TestPoolConfigDefaults(t *testing.T) {
	cfg := modbus.DefaultPoolConfig()

	tests := []struct {
		name     string
		got      interface{}
		expected interface{}
	}{
		{"MaxConnections", cfg.MaxConnections, 500},
		{"IdleTimeout", cfg.IdleTimeout, 5 * time.Minute},
		{"HealthCheckPeriod", cfg.HealthCheckPeriod, 30 * time.Second},
		{"ConnectionTimeout", cfg.ConnectionTimeout, 10 * time.Second},
		{"RetryAttempts", cfg.RetryAttempts, 3},
		{"RetryDelay", cfg.RetryDelay, 100 * time.Millisecond},
		{"CircuitBreakerName", cfg.CircuitBreakerName, "modbus-pool"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, tt.got)
			}
		})
	}
}

// TestPoolStatsStructure tests the PoolStats type.
func TestPoolStatsStructure(t *testing.T) {
	stats := modbus.PoolStats{
		TotalConnections:  50,
		ActiveConnections: 30,
		InUseConnections:  20,
		MaxConnections:    100,
	}

	if stats.TotalConnections != 50 {
		t.Errorf("expected TotalConnections 50, got %d", stats.TotalConnections)
	}
	if stats.ActiveConnections != 30 {
		t.Errorf("expected ActiveConnections 30, got %d", stats.ActiveConnections)
	}
	if stats.InUseConnections != 20 {
		t.Errorf("expected InUseConnections 20, got %d", stats.InUseConnections)
	}
	if stats.MaxConnections != 100 {
		t.Errorf("expected MaxConnections 100, got %d", stats.MaxConnections)
	}
}

// TestDeviceHealthStructure tests the DeviceHealth type.
func TestDeviceHealthStructure(t *testing.T) {
	health := modbus.DeviceHealth{
		DeviceID:           "device-001",
		Connected:          true,
		CircuitBreakerOpen: false,
		LastError:          nil,
	}

	if health.DeviceID != "device-001" {
		t.Errorf("expected DeviceID 'device-001', got %q", health.DeviceID)
	}
	if !health.Connected {
		t.Error("expected Connected to be true")
	}
	if health.CircuitBreakerOpen {
		t.Error("expected CircuitBreakerOpen to be false")
	}
	if health.LastError != nil {
		t.Error("expected LastError to be nil")
	}
}

// TestDeviceHealthWithError tests DeviceHealth with an error.
func TestDeviceHealthWithError(t *testing.T) {
	health := modbus.DeviceHealth{
		DeviceID:           "device-002",
		Connected:          false,
		CircuitBreakerOpen: true,
		LastError:          domain.ErrConnectionFailed,
	}

	if health.Connected {
		t.Error("expected Connected to be false")
	}
	if !health.CircuitBreakerOpen {
		t.Error("expected CircuitBreakerOpen to be true")
	}
	if health.LastError == nil {
		t.Error("expected LastError to be set")
	}
}

// TestDeviceStatsStructure tests the DeviceStats type.
func TestDeviceStatsStructure(t *testing.T) {
	stats := modbus.DeviceStats{
		DeviceID:       "device-001",
		ReadCount:      1000,
		WriteCount:     500,
		ErrorCount:     10,
		RetryCount:     25,
		AvgReadTimeMs:  1.5,
		AvgWriteTimeMs: 2.0,
		Connected:      true,
	}

	if stats.DeviceID != "device-001" {
		t.Errorf("expected DeviceID 'device-001', got %q", stats.DeviceID)
	}
	if stats.ReadCount != 1000 {
		t.Errorf("expected ReadCount 1000, got %d", stats.ReadCount)
	}
	if stats.WriteCount != 500 {
		t.Errorf("expected WriteCount 500, got %d", stats.WriteCount)
	}
	if stats.ErrorCount != 10 {
		t.Errorf("expected ErrorCount 10, got %d", stats.ErrorCount)
	}
	if stats.AvgReadTimeMs != 1.5 {
		t.Errorf("expected AvgReadTimeMs 1.5, got %f", stats.AvgReadTimeMs)
	}
}

// TestModbusAddressFormats tests various Modbus address formats.
func TestModbusAddressFormats(t *testing.T) {
	tests := []struct {
		name    string
		address string
	}{
		{"Standard TCP", "192.168.1.100:502"},
		{"Localhost", "127.0.0.1:502"},
		{"Custom port", "192.168.1.100:5502"},
		{"Hostname", "plc.factory.local:502"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := modbus.ClientConfig{Address: tt.address}
			if cfg.Address != tt.address {
				t.Errorf("expected address %q, got %q", tt.address, cfg.Address)
			}
		})
	}
}

// TestTimeoutSettings tests various timeout configurations.
func TestTimeoutSettings(t *testing.T) {
	tests := []struct {
		name        string
		timeout     time.Duration
		idleTimeout time.Duration
	}{
		{"Fast", 1 * time.Second, 10 * time.Second},
		{"Standard", 5 * time.Second, 30 * time.Second},
		{"Slow network", 15 * time.Second, 60 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := modbus.ClientConfig{
				Timeout:     tt.timeout,
				IdleTimeout: tt.idleTimeout,
			}
			if cfg.Timeout != tt.timeout {
				t.Errorf("expected Timeout %v, got %v", tt.timeout, cfg.Timeout)
			}
			if cfg.IdleTimeout != tt.idleTimeout {
				t.Errorf("expected IdleTimeout %v, got %v", tt.idleTimeout, cfg.IdleTimeout)
			}
		})
	}
}

// TestRetrySettings tests retry configuration.
func TestRetrySettings(t *testing.T) {
	tests := []struct {
		name       string
		maxRetries int
		retryDelay time.Duration
	}{
		{"No retry", 0, 0},
		{"Standard", 3, 100 * time.Millisecond},
		{"Aggressive", 5, 50 * time.Millisecond},
		{"Patient", 10, 1 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := modbus.ClientConfig{
				MaxRetries: tt.maxRetries,
				RetryDelay: tt.retryDelay,
			}
			if cfg.MaxRetries != tt.maxRetries {
				t.Errorf("expected MaxRetries %d, got %d", tt.maxRetries, cfg.MaxRetries)
			}
			if cfg.RetryDelay != tt.retryDelay {
				t.Errorf("expected RetryDelay %v, got %v", tt.retryDelay, cfg.RetryDelay)
			}
		})
	}
}

// TestModbusRegisterProtocolLimit tests the Modbus protocol limit of 125 registers.
func TestModbusRegisterProtocolLimit(t *testing.T) {
	const modbusMaxRegisters = 125

	cfg := modbus.DefaultBatchConfig()
	if cfg.MaxRegistersPerRead > modbusMaxRegisters {
		t.Errorf("MaxRegistersPerRead (%d) exceeds Modbus protocol limit (%d)",
			cfg.MaxRegistersPerRead, modbusMaxRegisters)
	}
}

// TestRegisterRangeCalculations tests register range calculations.
func TestRegisterRangeCalculations(t *testing.T) {
	tests := []struct {
		name     string
		start    uint16
		end      uint16
		expected int
	}{
		{"Single register", 100, 100, 1},
		{"Small range", 100, 104, 5},
		{"Medium range", 0, 99, 100},
		{"Max safe range", 0, 124, 125},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rr := modbus.RegisterRange{
				StartAddress: tt.start,
				EndAddress:   tt.end,
			}
			count := int(rr.EndAddress-rr.StartAddress) + 1
			if count != tt.expected {
				t.Errorf("expected count %d, got %d", tt.expected, count)
			}
		})
	}
}
