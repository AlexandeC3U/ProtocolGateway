// Package domain contains core business entities.
package domain

import "errors"

// Device configuration errors.
var (
	ErrDeviceIDRequired     = errors.New("device ID is required")
	ErrDeviceNameRequired   = errors.New("device name is required")
	ErrProtocolRequired     = errors.New("protocol is required")
	ErrNoTagsDefined        = errors.New("at least one tag must be defined")
	ErrPollIntervalTooShort = errors.New("poll interval must be at least 100ms")
	ErrUNSPrefixRequired    = errors.New("UNS prefix is required")
)

// Connection errors.
var (
	ErrConnectionFailed   = errors.New("connection failed")
	ErrConnectionTimeout  = errors.New("connection timeout")
	ErrConnectionClosed   = errors.New("connection closed")
	ErrConnectionReset    = errors.New("connection reset by peer")
	ErrMaxRetriesExceeded = errors.New("maximum retry attempts exceeded")
	ErrCircuitBreakerOpen = errors.New("circuit breaker is open")
	ErrPoolExhausted      = errors.New("connection pool exhausted")
	ErrInvalidSlaveID     = errors.New("invalid slave ID")
)

// Read/Write errors.
var (
	ErrReadFailed          = errors.New("read operation failed")
	ErrWriteFailed         = errors.New("write operation failed")
	ErrInvalidAddress      = errors.New("invalid register address")
	ErrInvalidDataLength   = errors.New("invalid data length")
	ErrInvalidDataType     = errors.New("invalid data type")
	ErrInvalidRegisterType = errors.New("invalid register type")
)

// Modbus-specific errors.
var (
	ErrModbusIllegalFunction        = errors.New("modbus: illegal function")
	ErrModbusIllegalAddress         = errors.New("modbus: illegal data address")
	ErrModbusIllegalValue           = errors.New("modbus: illegal data value")
	ErrModbusDeviceFailure          = errors.New("modbus: slave device failure")
	ErrModbusAcknowledge            = errors.New("modbus: acknowledge - long operation in progress")
	ErrModbusBusy                   = errors.New("modbus: slave device busy")
	ErrModbusNegativeAck            = errors.New("modbus: negative acknowledge")
	ErrModbusMemoryParityError      = errors.New("modbus: memory parity error")
	ErrModbusGatewayPathUnavailable = errors.New("modbus: gateway path unavailable")
	ErrModbusGatewayTargetFailed    = errors.New("modbus: gateway target device failed to respond")
	ErrModbusProtocolLimit          = errors.New("modbus: protocol limit exceeded")
	ErrInvalidRegisterCount         = errors.New("modbus: invalid register count")
)

// MQTT errors.
var (
	ErrMQTTConnectionFailed = errors.New("MQTT connection failed")
	ErrMQTTPublishFailed    = errors.New("MQTT publish failed")
	ErrMQTTNotConnected     = errors.New("MQTT client not connected")
	ErrMQTTSubscribeFailed  = errors.New("MQTT subscribe failed")
)

// OPC UA specific errors.
var (
	ErrOPCUAInvalidNodeID      = errors.New("opcua: invalid node ID")
	ErrOPCUASubscriptionFailed = errors.New("opcua: subscription failed")
	ErrOPCUABadStatus          = errors.New("opcua: bad status code")
	ErrOPCUASecurityFailed     = errors.New("opcua: security negotiation failed")
	ErrOPCUASessionExpired     = errors.New("opcua: session expired")
	ErrOPCUABrowseFailed       = errors.New("opcua: browse failed")
	ErrOPCUANodeNotFound       = errors.New("opcua: node not found")
	ErrOPCUAAccessDenied       = errors.New("opcua: access denied")
	ErrOPCUAWriteNotPermitted  = errors.New("opcua: write not permitted")
)

// S7 (Siemens) specific errors.
var (
	ErrS7ConnectionFailed      = errors.New("s7: connection failed")
	ErrS7InvalidAddress        = errors.New("s7: invalid address format")
	ErrS7InvalidDBNumber       = errors.New("s7: invalid data block number")
	ErrS7InvalidArea           = errors.New("s7: invalid memory area")
	ErrS7InvalidOffset         = errors.New("s7: invalid offset")
	ErrS7ReadFailed            = errors.New("s7: read operation failed")
	ErrS7WriteFailed           = errors.New("s7: write operation failed")
	ErrS7CPUError              = errors.New("s7: CPU error")
	ErrS7PDUSizeMismatch       = errors.New("s7: PDU size mismatch")
	ErrS7ItemNotAvailable      = errors.New("s7: item not available")
	ErrS7AddressOutOfRange     = errors.New("s7: address out of range")
	ErrS7WriteDataSizeMismatch = errors.New("s7: write data size mismatch")
	ErrS7ObjectNotExist        = errors.New("s7: object does not exist")
	ErrS7HardwareFault         = errors.New("s7: hardware fault")
	ErrS7AccessingNotAllowed   = errors.New("s7: accessing not allowed")
)

// Write operation errors.
var (
	ErrTagNotWritable    = errors.New("tag is not writable")
	ErrInvalidWriteValue = errors.New("invalid value for write operation")
	ErrWriteTimeout      = errors.New("write operation timed out")
)

// Service errors.
var (
	ErrServiceNotStarted    = errors.New("service not started")
	ErrServiceStopped       = errors.New("service has been stopped")
	ErrServiceOverloaded    = errors.New("service overloaded - brownout mode active")
	ErrDeviceNotFound       = errors.New("device not found")
	ErrDeviceExists         = errors.New("device already exists")
	ErrTagNotFound          = errors.New("tag not found")
	ErrInvalidConfig        = errors.New("invalid configuration")
	ErrProtocolNotSupported = errors.New("protocol not supported")
)

// ModbusExceptionToError converts a Modbus exception code to a domain error.
func ModbusExceptionToError(code byte) error {
	switch code {
	case 0x01:
		return ErrModbusIllegalFunction
	case 0x02:
		return ErrModbusIllegalAddress
	case 0x03:
		return ErrModbusIllegalValue
	case 0x04:
		return ErrModbusDeviceFailure
	case 0x05:
		return ErrModbusAcknowledge
	case 0x06:
		return ErrModbusBusy
	case 0x07:
		return ErrModbusNegativeAck
	case 0x08:
		return ErrModbusMemoryParityError
	case 0x0A:
		return ErrModbusGatewayPathUnavailable
	case 0x0B:
		return ErrModbusGatewayTargetFailed
	default:
		return ErrReadFailed
	}
}
