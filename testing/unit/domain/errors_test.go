package domain_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/nexus-edge/protocol-gateway/internal/domain"
)

func TestDeviceConfigurationErrors(t *testing.T) {
	errs := []struct {
		err      error
		expected string
	}{
		{domain.ErrDeviceIDRequired, "device ID is required"},
		{domain.ErrDeviceNameRequired, "device name is required"},
		{domain.ErrProtocolRequired, "protocol is required"},
		{domain.ErrNoTagsDefined, "at least one tag must be defined"},
		{domain.ErrPollIntervalTooShort, "poll interval must be at least 100ms"},
		{domain.ErrUNSPrefixRequired, "UNS prefix is required"},
	}

	for _, tt := range errs {
		if tt.err.Error() != tt.expected {
			t.Errorf("expected '%s', got '%s'", tt.expected, tt.err.Error())
		}
	}
}

func TestConnectionErrors(t *testing.T) {
	errs := []error{
		domain.ErrConnectionFailed,
		domain.ErrConnectionTimeout,
		domain.ErrConnectionClosed,
		domain.ErrConnectionReset,
		domain.ErrMaxRetriesExceeded,
		domain.ErrCircuitBreakerOpen,
		domain.ErrPoolExhausted,
		domain.ErrInvalidSlaveID,
	}

	for _, err := range errs {
		if err == nil {
			t.Error("expected error to be non-nil")
		}
		if err.Error() == "" {
			t.Error("expected error message to be non-empty")
		}
	}
}

func TestModbusErrors(t *testing.T) {
	errs := []struct {
		err      error
		contains string
	}{
		{domain.ErrModbusIllegalFunction, "illegal function"},
		{domain.ErrModbusIllegalAddress, "illegal data address"},
		{domain.ErrModbusIllegalValue, "illegal data value"},
		{domain.ErrModbusDeviceFailure, "device failure"},
		{domain.ErrModbusBusy, "busy"},
		{domain.ErrModbusProtocolLimit, "protocol limit"},
	}

	for _, tt := range errs {
		if !containsString(tt.err.Error(), tt.contains) {
			t.Errorf("expected error '%s' to contain '%s'", tt.err.Error(), tt.contains)
		}
	}
}

func TestOPCUAErrors(t *testing.T) {
	errs := []struct {
		err      error
		contains string
	}{
		{domain.ErrOPCUAInvalidNodeID, "invalid node ID"},
		{domain.ErrOPCUASubscriptionFailed, "subscription failed"},
		{domain.ErrOPCUABadStatus, "bad status"},
		{domain.ErrOPCUASecurityFailed, "security"},
		{domain.ErrOPCUASessionExpired, "session expired"},
		{domain.ErrOPCUABrowseFailed, "browse failed"},
		{domain.ErrOPCUANodeNotFound, "node not found"},
		{domain.ErrOPCUAAccessDenied, "access denied"},
		{domain.ErrOPCUAWriteNotPermitted, "write not permitted"},
	}

	for _, tt := range errs {
		t.Run(tt.contains, func(t *testing.T) {
			if !containsString(tt.err.Error(), tt.contains) {
				t.Errorf("expected error '%s' to contain '%s'", tt.err.Error(), tt.contains)
			}
		})
	}
}

func TestS7Errors(t *testing.T) {
	errs := []struct {
		err      error
		contains string
	}{
		{domain.ErrS7ConnectionFailed, "connection failed"},
		{domain.ErrS7InvalidAddress, "invalid address"},
		{domain.ErrS7InvalidDBNumber, "data block"},
		{domain.ErrS7InvalidArea, "memory area"},
		{domain.ErrS7ReadFailed, "read"},
		{domain.ErrS7WriteFailed, "write"},
		{domain.ErrS7CPUError, "CPU error"},
		{domain.ErrS7ItemNotAvailable, "not available"},
		{domain.ErrS7ObjectNotExist, "does not exist"},
		{domain.ErrS7HardwareFault, "hardware fault"},
	}

	for _, tt := range errs {
		t.Run(tt.contains, func(t *testing.T) {
			if !containsString(tt.err.Error(), tt.contains) {
				t.Errorf("expected error '%s' to contain '%s'", tt.err.Error(), tt.contains)
			}
		})
	}
}

func TestMQTTErrors(t *testing.T) {
	errs := []error{
		domain.ErrMQTTConnectionFailed,
		domain.ErrMQTTPublishFailed,
		domain.ErrMQTTNotConnected,
		domain.ErrMQTTSubscribeFailed,
	}

	for _, err := range errs {
		if !containsString(err.Error(), "MQTT") {
			t.Errorf("expected MQTT error to contain 'MQTT': %s", err.Error())
		}
	}
}

func TestError_Is(t *testing.T) {
	// Test that errors.Is works correctly
	wrapped := fmt.Errorf("operation failed: %w", domain.ErrConnectionTimeout)

	if !errors.Is(wrapped, domain.ErrConnectionTimeout) {
		t.Error("errors.Is should match wrapped error")
	}

	if errors.Is(wrapped, domain.ErrConnectionFailed) {
		t.Error("errors.Is should not match different error")
	}
}

func TestError_Wrapping(t *testing.T) {
	// Test proper error wrapping pattern
	baseErr := domain.ErrReadFailed
	contextErr := fmt.Errorf("reading tag 'temperature': %w", baseErr)
	topErr := fmt.Errorf("device 'plc-001': %w", contextErr)

	// Should be able to unwrap to the base error
	if !errors.Is(topErr, domain.ErrReadFailed) {
		t.Error("should be able to unwrap to base error")
	}

	// Check error chain
	var target error = domain.ErrReadFailed
	if !errors.Is(topErr, target) {
		t.Error("error chain should include base error")
	}
}

func TestWriteErrors(t *testing.T) {
	errs := []struct {
		err      error
		contains string
	}{
		{domain.ErrTagNotWritable, "not writable"},
		{domain.ErrInvalidWriteValue, "invalid value"},
		{domain.ErrWriteTimeout, "timed out"},
	}

	for _, tt := range errs {
		t.Run(tt.contains, func(t *testing.T) {
			if !containsString(tt.err.Error(), tt.contains) {
				t.Errorf("expected error '%s' to contain '%s'", tt.err.Error(), tt.contains)
			}
		})
	}
}

func TestReadWriteErrors(t *testing.T) {
	errs := []error{
		domain.ErrReadFailed,
		domain.ErrWriteFailed,
		domain.ErrInvalidAddress,
		domain.ErrInvalidDataLength,
		domain.ErrInvalidDataType,
		domain.ErrInvalidRegisterType,
	}

	for _, err := range errs {
		if err == nil {
			t.Error("error should not be nil")
		}
		if err.Error() == "" {
			t.Error("error message should not be empty")
		}
	}
}

func TestError_Uniqueness(t *testing.T) {
	// Ensure all errors are unique
	allErrors := []error{
		domain.ErrConnectionFailed,
		domain.ErrConnectionTimeout,
		domain.ErrConnectionClosed,
		domain.ErrReadFailed,
		domain.ErrWriteFailed,
		domain.ErrModbusIllegalFunction,
		domain.ErrOPCUAInvalidNodeID,
		domain.ErrS7ConnectionFailed,
		domain.ErrMQTTConnectionFailed,
	}

	seen := make(map[string]bool)
	for _, err := range allErrors {
		msg := err.Error()
		if seen[msg] {
			t.Errorf("duplicate error message: %s", msg)
		}
		seen[msg] = true
	}
}

// Helper function
func containsString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
