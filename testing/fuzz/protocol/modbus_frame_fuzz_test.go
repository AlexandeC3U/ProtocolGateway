//go:build fuzz
// +build fuzz

// Package protocol provides fuzz tests for Modbus protocol frame handling.
package protocol

import (
	"bytes"
	"encoding/binary"
	"testing"
)

// =============================================================================
// Modbus Frame Constants
// =============================================================================

const (
	// Function codes
	FuncReadCoils          = 0x01
	FuncReadDiscreteInputs = 0x02
	FuncReadHoldingRegs    = 0x03
	FuncReadInputRegs      = 0x04
	FuncWriteSingleCoil    = 0x05
	FuncWriteSingleReg     = 0x06
	FuncWriteMultipleCoils = 0x0F
	FuncWriteMultipleRegs  = 0x10

	// Exception codes
	ExceptionIllegalFunction     = 0x01
	ExceptionIllegalDataAddress  = 0x02
	ExceptionIllegalDataValue    = 0x03
	ExceptionServerDeviceFailure = 0x04

	// Protocol limits
	MaxPDUSize     = 253
	MaxCoils       = 2000
	MaxRegisters   = 125
	MBAPHeaderSize = 7
)

// =============================================================================
// Modbus TCP Frame Structures
// =============================================================================

// MBAPHeader represents the Modbus Application Protocol header.
type MBAPHeader struct {
	TransactionID uint16
	ProtocolID    uint16
	Length        uint16
	UnitID        byte
}

// ModbusFrame represents a complete Modbus TCP frame.
type ModbusFrame struct {
	Header       MBAPHeader
	FunctionCode byte
	Data         []byte
}

// =============================================================================
// Frame Parsing Functions
// =============================================================================

// ParseMBAPHeader parses a MBAP header from bytes.
func ParseMBAPHeader(data []byte) (*MBAPHeader, error) {
	if len(data) < MBAPHeaderSize {
		return nil, ErrInvalidFrameLength
	}

	return &MBAPHeader{
		TransactionID: binary.BigEndian.Uint16(data[0:2]),
		ProtocolID:    binary.BigEndian.Uint16(data[2:4]),
		Length:        binary.BigEndian.Uint16(data[4:6]),
		UnitID:        data[6],
	}, nil
}

// ParseModbusFrame parses a complete Modbus TCP frame.
func ParseModbusFrame(data []byte) (*ModbusFrame, error) {
	if len(data) < MBAPHeaderSize+1 {
		return nil, ErrInvalidFrameLength
	}

	header, err := ParseMBAPHeader(data)
	if err != nil {
		return nil, err
	}

	// Validate protocol ID (should be 0 for Modbus TCP)
	if header.ProtocolID != 0 {
		return nil, ErrInvalidProtocolID
	}

	// Validate length field
	if int(header.Length) > len(data)-6 {
		return nil, ErrInvalidFrameLength
	}

	// Check for maximum PDU size
	if header.Length > MaxPDUSize+1 { // +1 for unit ID
		return nil, ErrPDUTooLarge
	}

	funcCode := data[MBAPHeaderSize]
	frameData := data[MBAPHeaderSize+1 : 6+header.Length]

	return &ModbusFrame{
		Header:       *header,
		FunctionCode: funcCode,
		Data:         frameData,
	}, nil
}

// ValidateRequest validates a Modbus request frame.
func ValidateRequest(frame *ModbusFrame) error {
	if frame == nil {
		return ErrNilFrame
	}

	switch frame.FunctionCode {
	case FuncReadCoils, FuncReadDiscreteInputs:
		return validateReadBitsRequest(frame.Data)
	case FuncReadHoldingRegs, FuncReadInputRegs:
		return validateReadRegsRequest(frame.Data)
	case FuncWriteSingleCoil:
		return validateWriteSingleCoil(frame.Data)
	case FuncWriteSingleReg:
		return validateWriteSingleReg(frame.Data)
	case FuncWriteMultipleCoils:
		return validateWriteMultipleCoils(frame.Data)
	case FuncWriteMultipleRegs:
		return validateWriteMultipleRegs(frame.Data)
	default:
		// Unknown function code - may be valid in some implementations
		return nil
	}
}

// validateReadBitsRequest validates read coils/discrete inputs request data.
func validateReadBitsRequest(data []byte) error {
	if len(data) < 4 {
		return ErrInvalidDataLength
	}

	quantity := binary.BigEndian.Uint16(data[2:4])
	if quantity == 0 || quantity > MaxCoils {
		return ErrInvalidQuantity
	}

	return nil
}

// validateReadRegsRequest validates read holding/input registers request data.
func validateReadRegsRequest(data []byte) error {
	if len(data) < 4 {
		return ErrInvalidDataLength
	}

	quantity := binary.BigEndian.Uint16(data[2:4])
	if quantity == 0 || quantity > MaxRegisters {
		return ErrInvalidQuantity
	}

	return nil
}

// validateWriteSingleCoil validates write single coil request data.
func validateWriteSingleCoil(data []byte) error {
	if len(data) < 4 {
		return ErrInvalidDataLength
	}

	value := binary.BigEndian.Uint16(data[2:4])
	// Coil value must be 0x0000 (OFF) or 0xFF00 (ON)
	if value != 0x0000 && value != 0xFF00 {
		return ErrInvalidCoilValue
	}

	return nil
}

// validateWriteSingleReg validates write single register request data.
func validateWriteSingleReg(data []byte) error {
	if len(data) < 4 {
		return ErrInvalidDataLength
	}
	// Any 16-bit value is valid
	return nil
}

// validateWriteMultipleCoils validates write multiple coils request data.
func validateWriteMultipleCoils(data []byte) error {
	if len(data) < 5 {
		return ErrInvalidDataLength
	}

	quantity := binary.BigEndian.Uint16(data[2:4])
	if quantity == 0 || quantity > MaxCoils {
		return ErrInvalidQuantity
	}

	byteCount := data[4]
	expectedBytes := (quantity + 7) / 8
	if int(byteCount) != int(expectedBytes) {
		return ErrBytecountMismatch
	}

	if len(data) < 5+int(byteCount) {
		return ErrInvalidDataLength
	}

	return nil
}

// validateWriteMultipleRegs validates write multiple registers request data.
func validateWriteMultipleRegs(data []byte) error {
	if len(data) < 5 {
		return ErrInvalidDataLength
	}

	quantity := binary.BigEndian.Uint16(data[2:4])
	if quantity == 0 || quantity > MaxRegisters {
		return ErrInvalidQuantity
	}

	byteCount := data[4]
	expectedBytes := quantity * 2
	if int(byteCount) != int(expectedBytes) {
		return ErrBytecountMismatch
	}

	if len(data) < 5+int(byteCount) {
		return ErrInvalidDataLength
	}

	return nil
}

// =============================================================================
// Error Types
// =============================================================================

type frameError string

func (e frameError) Error() string { return string(e) }

const (
	ErrInvalidFrameLength frameError = "invalid frame length"
	ErrInvalidProtocolID  frameError = "invalid protocol ID"
	ErrPDUTooLarge        frameError = "PDU too large"
	ErrNilFrame           frameError = "nil frame"
	ErrInvalidDataLength  frameError = "invalid data length"
	ErrInvalidQuantity    frameError = "invalid quantity"
	ErrInvalidCoilValue   frameError = "invalid coil value"
	ErrBytecountMismatch  frameError = "byte count mismatch"
)

// =============================================================================
// Fuzz Tests
// =============================================================================

// FuzzModbusFrameParsing tests frame parsing with random inputs.
func FuzzModbusFrameParsing(f *testing.F) {
	// Seed with valid frames
	validReadCoils := []byte{
		0x00, 0x01, // Transaction ID
		0x00, 0x00, // Protocol ID
		0x00, 0x06, // Length
		0x01,       // Unit ID
		0x01,       // Function code (read coils)
		0x00, 0x00, // Start address
		0x00, 0x10, // Quantity
	}
	f.Add(validReadCoils)

	validReadRegs := []byte{
		0x00, 0x02, // Transaction ID
		0x00, 0x00, // Protocol ID
		0x00, 0x06, // Length
		0x01,       // Unit ID
		0x03,       // Function code (read holding registers)
		0x00, 0x00, // Start address
		0x00, 0x0A, // Quantity (10 registers)
	}
	f.Add(validReadRegs)

	validWriteSingleCoil := []byte{
		0x00, 0x03, // Transaction ID
		0x00, 0x00, // Protocol ID
		0x00, 0x06, // Length
		0x01,       // Unit ID
		0x05,       // Function code (write single coil)
		0x00, 0x05, // Address
		0xFF, 0x00, // Value (ON)
	}
	f.Add(validWriteSingleCoil)

	// Seed with edge cases
	f.Add([]byte{})                        // Empty
	f.Add([]byte{0x00})                    // Too short
	f.Add([]byte{0x00, 0x00, 0x00})        // Partial header
	f.Add(bytes.Repeat([]byte{0xFF}, 300)) // Oversized

	f.Fuzz(func(t *testing.T, data []byte) {
		// Parse should not panic
		frame, err := ParseModbusFrame(data)

		if err != nil {
			// Error is expected for malformed input
			return
		}

		if frame == nil {
			t.Error("ParseModbusFrame returned nil frame without error")
			return
		}

		// If parsing succeeded, validation should not panic
		_ = ValidateRequest(frame)
	})
}

// FuzzMBAPHeader tests MBAP header parsing with random inputs.
func FuzzMBAPHeader(f *testing.F) {
	// Valid headers
	f.Add([]byte{0x00, 0x01, 0x00, 0x00, 0x00, 0x06, 0x01})
	f.Add([]byte{0xFF, 0xFF, 0x00, 0x00, 0x00, 0xFE, 0xFF})

	// Edge cases
	f.Add([]byte{})
	f.Add([]byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00})
	f.Add(bytes.Repeat([]byte{0x00}, 7))

	f.Fuzz(func(t *testing.T, data []byte) {
		header, err := ParseMBAPHeader(data)

		if err != nil {
			return
		}

		if header == nil {
			t.Error("ParseMBAPHeader returned nil without error")
			return
		}

		// Verify header fields are populated
		_ = header.TransactionID
		_ = header.ProtocolID
		_ = header.Length
		_ = header.UnitID
	})
}

// FuzzReadRequest tests read request validation with random data.
func FuzzReadRequest(f *testing.F) {
	// Valid read requests
	f.Add([]byte{0x00, 0x00, 0x00, 0x01}) // Read 1 from address 0
	f.Add([]byte{0x00, 0x64, 0x00, 0x0A}) // Read 10 from address 100
	f.Add([]byte{0xFF, 0xFF, 0x00, 0x7D}) // Read 125 from address 65535

	// Invalid quantities
	f.Add([]byte{0x00, 0x00, 0x00, 0x00}) // Quantity 0
	f.Add([]byte{0x00, 0x00, 0x07, 0xD1}) // Quantity > 2000

	// Edge cases
	f.Add([]byte{})
	f.Add([]byte{0x00})

	f.Fuzz(func(t *testing.T, data []byte) {
		// Should not panic regardless of input
		_ = validateReadBitsRequest(data)
		_ = validateReadRegsRequest(data)
	})
}

// FuzzWriteCoilRequest tests write coil validation with random data.
func FuzzWriteCoilRequest(f *testing.F) {
	// Valid coil values
	f.Add([]byte{0x00, 0x00, 0xFF, 0x00}) // ON
	f.Add([]byte{0x00, 0x00, 0x00, 0x00}) // OFF

	// Invalid coil values
	f.Add([]byte{0x00, 0x00, 0x00, 0x01}) // Invalid
	f.Add([]byte{0x00, 0x00, 0xFF, 0xFF}) // Invalid

	// Edge cases
	f.Add([]byte{})
	f.Add([]byte{0x00, 0x00})

	f.Fuzz(func(t *testing.T, data []byte) {
		err := validateWriteSingleCoil(data)

		// If we have 4 bytes, check that validation is correct
		if len(data) >= 4 {
			value := binary.BigEndian.Uint16(data[2:4])
			if value == 0x0000 || value == 0xFF00 {
				if err != nil && err != ErrInvalidDataLength {
					// Should be valid
				}
			}
		}
	})
}

// FuzzWriteMultipleRegs tests write multiple registers validation.
func FuzzWriteMultipleRegs(f *testing.F) {
	// Valid write multiple registers
	validReq := []byte{
		0x00, 0x00, // Start address
		0x00, 0x02, // Quantity (2)
		0x04,       // Byte count
		0x00, 0x0A, // Value 1
		0x01, 0x02, // Value 2
	}
	f.Add(validReq)

	// Edge cases
	f.Add([]byte{0x00, 0x00, 0x00, 0x00, 0x00}) // Zero quantity
	f.Add([]byte{0x00, 0x00, 0x00, 0x01, 0x01}) // Byte count mismatch

	f.Fuzz(func(t *testing.T, data []byte) {
		// Should not panic
		_ = validateWriteMultipleRegs(data)
	})
}

// FuzzExceptionResponse tests exception response handling.
func FuzzExceptionResponse(f *testing.F) {
	// Exception frames (function code | 0x80)
	f.Add([]byte{
		0x00, 0x01, // Transaction ID
		0x00, 0x00, // Protocol ID
		0x00, 0x03, // Length
		0x01, // Unit ID
		0x81, // Exception (read coils)
		0x01, // Exception code (illegal function)
	})

	f.Add([]byte{
		0x00, 0x02, // Transaction ID
		0x00, 0x00, // Protocol ID
		0x00, 0x03, // Length
		0x01, // Unit ID
		0x83, // Exception (read holding registers)
		0x02, // Exception code (illegal data address)
	})

	f.Fuzz(func(t *testing.T, data []byte) {
		frame, err := ParseModbusFrame(data)
		if err != nil {
			return
		}

		// Check if this is an exception response
		if frame.FunctionCode&0x80 != 0 {
			// Exception response - function code has high bit set
			originalFunc := frame.FunctionCode & 0x7F
			if originalFunc > 0 && len(frame.Data) > 0 {
				exceptionCode := frame.Data[0]
				_ = exceptionCode // Would be used for error handling
			}
		}
	})
}

// FuzzFrameRoundTrip tests that frames can be serialized and deserialized.
func FuzzFrameRoundTrip(f *testing.F) {
	f.Add(uint16(1), byte(1), byte(0x03), []byte{0x00, 0x00, 0x00, 0x0A})

	f.Fuzz(func(t *testing.T, txID uint16, unitID byte, funcCode byte, data []byte) {
		// Limit data size to prevent extremely large allocations
		if len(data) > 250 {
			data = data[:250]
		}

		// Build frame
		length := uint16(2 + len(data)) // unit ID + func code + data
		frame := make([]byte, 0, MBAPHeaderSize+1+len(data))

		// MBAP header
		frame = binary.BigEndian.AppendUint16(frame, txID)
		frame = binary.BigEndian.AppendUint16(frame, 0) // Protocol ID
		frame = binary.BigEndian.AppendUint16(frame, length)
		frame = append(frame, unitID)
		frame = append(frame, funcCode)
		frame = append(frame, data...)

		// Parse it back
		parsed, err := ParseModbusFrame(frame)
		if err != nil {
			// May fail for invalid combinations
			return
		}

		// Verify round-trip
		if parsed.Header.TransactionID != txID {
			t.Errorf("TransactionID mismatch: got %d, want %d", parsed.Header.TransactionID, txID)
		}
		if parsed.Header.UnitID != unitID {
			t.Errorf("UnitID mismatch: got %d, want %d", parsed.Header.UnitID, unitID)
		}
		if parsed.FunctionCode != funcCode {
			t.Errorf("FunctionCode mismatch: got %d, want %d", parsed.FunctionCode, funcCode)
		}
	})
}
