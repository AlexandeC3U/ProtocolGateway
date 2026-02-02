// Package testutil provides shared utilities for testing.
package testutil

import (
	"context"
	"testing"
	"time"

	"github.com/nexus-edge/protocol-gateway/internal/domain"
)

// TestTimeout is the default timeout for test operations.
const TestTimeout = 5 * time.Second

// ShortTimeout is for operations expected to complete quickly.
const ShortTimeout = 100 * time.Millisecond

// ContextWithTimeout returns a context with the default test timeout.
func ContextWithTimeout(t *testing.T) (context.Context, context.CancelFunc) {
	t.Helper()
	return context.WithTimeout(context.Background(), TestTimeout)
}

// ContextWithShortTimeout returns a context with a short timeout.
func ContextWithShortTimeout(t *testing.T) (context.Context, context.CancelFunc) {
	t.Helper()
	return context.WithTimeout(context.Background(), ShortTimeout)
}

// RequireNoError fails the test immediately if err is not nil.
func RequireNoError(t *testing.T, err error, msgAndArgs ...interface{}) {
	t.Helper()
	if err != nil {
		if len(msgAndArgs) > 0 {
			t.Fatalf("unexpected error: %v - %v", err, msgAndArgs)
		}
		t.Fatalf("unexpected error: %v", err)
	}
}

// RequireError fails the test if err is nil.
func RequireError(t *testing.T, err error, msgAndArgs ...interface{}) {
	t.Helper()
	if err == nil {
		if len(msgAndArgs) > 0 {
			t.Fatalf("expected error but got nil - %v", msgAndArgs)
		}
		t.Fatalf("expected error but got nil")
	}
}

// AssertEqual checks if two values are equal.
func AssertEqual(t *testing.T, expected, actual interface{}, msgAndArgs ...interface{}) {
	t.Helper()
	if expected != actual {
		if len(msgAndArgs) > 0 {
			t.Errorf("expected %v, got %v - %v", expected, actual, msgAndArgs)
		} else {
			t.Errorf("expected %v, got %v", expected, actual)
		}
	}
}

// AssertTrue checks if a condition is true.
func AssertTrue(t *testing.T, condition bool, msgAndArgs ...interface{}) {
	t.Helper()
	if !condition {
		if len(msgAndArgs) > 0 {
			t.Errorf("expected true - %v", msgAndArgs)
		} else {
			t.Errorf("expected true")
		}
	}
}

// AssertFalse checks if a condition is false.
func AssertFalse(t *testing.T, condition bool, msgAndArgs ...interface{}) {
	t.Helper()
	if condition {
		if len(msgAndArgs) > 0 {
			t.Errorf("expected false - %v", msgAndArgs)
		} else {
			t.Errorf("expected false")
		}
	}
}

// AssertNil checks if a value is nil.
func AssertNil(t *testing.T, value interface{}, msgAndArgs ...interface{}) {
	t.Helper()
	if value != nil {
		if len(msgAndArgs) > 0 {
			t.Errorf("expected nil, got %v - %v", value, msgAndArgs)
		} else {
			t.Errorf("expected nil, got %v", value)
		}
	}
}

// AssertNotNil checks if a value is not nil.
func AssertNotNil(t *testing.T, value interface{}, msgAndArgs ...interface{}) {
	t.Helper()
	if value == nil {
		if len(msgAndArgs) > 0 {
			t.Errorf("expected non-nil value - %v", msgAndArgs)
		} else {
			t.Errorf("expected non-nil value")
		}
	}
}

// AssertErrorIs checks if an error matches a target error.
func AssertErrorIs(t *testing.T, err, target error) {
	t.Helper()
	if err == nil {
		t.Errorf("expected error %v, got nil", target)
		return
	}
	// Use errors.Is for proper error chain checking
	// Simplified version for now
	if err.Error() != target.Error() {
		t.Errorf("expected error %v, got %v", target, err)
	}
}

// WaitForCondition waits for a condition to become true.
func WaitForCondition(t *testing.T, condition func() bool, timeout time.Duration, msg string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("condition not met within %v: %s", timeout, msg)
}

// EventuallyTrue waits for condition to become true, with polling.
func EventuallyTrue(t *testing.T, condition func() bool, timeout, interval time.Duration) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return true
		}
		time.Sleep(interval)
	}
	return false
}

// MakeDataPoint creates a test DataPoint with sensible defaults.
func MakeDataPoint(deviceID, tagID string, value interface{}) *domain.DataPoint {
	return &domain.DataPoint{
		DeviceID:  deviceID,
		TagID:     tagID,
		Topic:     "test/" + deviceID + "/" + tagID,
		Value:     value,
		Quality:   domain.QualityGood,
		Timestamp: time.Now(),
	}
}

// MakeTag creates a test Tag with sensible defaults.
func MakeTag(id, name string, dataType domain.DataType) *domain.Tag {
	return &domain.Tag{
		ID:       id,
		Name:     name,
		DataType: dataType,
		Enabled:  true,
	}
}

// MakeDevice creates a test Device with sensible defaults.
func MakeDevice(id, name string, protocol domain.Protocol) *domain.Device {
	return &domain.Device{
		ID:       id,
		Name:     name,
		Protocol: protocol,
		Enabled:  true,
	}
}
