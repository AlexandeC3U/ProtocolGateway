//go:build fuzz
// +build fuzz

// Package protocol provides fuzz tests for S7 protocol packet handling.
package protocol

import (
	"bytes"
	"encoding/binary"
	"testing"
)

// =============================================================================
// S7 Protocol Constants
// =============================================================================

const (
	// TPKT Header (ISO-on-TCP, RFC 1006)
	TPKTVersion    = 0x03
	TPKTReserved   = 0x00
	TPKTHeaderSize = 4

	// COTP (Connection-Oriented Transport Protocol)
	COTPHeaderSize  = 3 // Minimum for data TPDU
	COTPDataTDPU    = 0xF0
	COTPConnectTDPU = 0xE0
	COTPAcceptTDPU  = 0xD0

	// S7 PDU types
	S7PDUJobRequest = 0x01
	S7PDUAckData    = 0x03
	S7PDUUserData   = 0x07

	// S7 Function codes
	S7FuncReadVar    = 0x04
	S7FuncWriteVar   = 0x05
	S7FuncSetupComm  = 0xF0
	S7FuncReadSZL    = 0x44 // System Status List

	// S7 Area codes
	S7AreaDB = 0x84 // Data Blocks
	S7AreaM  = 0x83 // Merkers
	S7AreaI  = 0x81 // Inputs
	S7AreaQ  = 0x82 // Outputs
	S7AreaT  = 0x1D // Timers
	S7AreaC  = 0x1C // Counters

	// S7 Word Length constants
	S7WLBit   = 0x01
	S7WLByte  = 0x02
	S7WLWord  = 0x04
	S7WLDWord = 0x06
	S7WLReal  = 0x08

	// S7 Protocol limits
	S7MaxPDUSize       = 480
	S7MaxMultiReadItems = 20
	S7MaxDBNumber      = 65535
)

// =============================================================================
// S7 Packet Structures
// =============================================================================

// TPKTHeader represents the TPKT header (ISO-on-TCP).
type TPKTHeader struct {
	Version  byte
	Reserved byte
	Length   uint16 // Total packet length including TPKT header
}

// COTPHeader represents the COTP header for data transfer.
type COTPHeader struct {
	Length byte // Header length (excluding length byte itself)
	PDUType byte
	TPDUNR byte // TPDU number + EOT flag
}

// S7Header represents the S7 communication header.
type S7Header struct {
	ProtocolID   byte   // Always 0x32
	PDUType      byte   // Job(1), Ack(2), AckData(3), UserData(7)
	Reserved     uint16 // Always 0x0000
	PDUReference uint16
	ParamLength  uint16
	DataLength   uint16
	// For AckData, additional fields: ErrorClass, ErrorCode
}

// S7ReadItem represents a single item in a multi-read request.
type S7ReadItem struct {
	SpecType      byte   // 0x12 = variable specification
	LengthOfSpec  byte   // Remaining length (10 for S7Any)
	SyntaxID      byte   // 0x10 = S7Any
	TransportSize byte   // Word length code
	Length        uint16 // Number of elements
	DBNumber      uint16
	Area          byte
	Address       [3]byte // Byte offset * 8 + bit offset, 3 bytes big-endian
}

// =============================================================================
// Parsing Functions
// =============================================================================

// ParseTPKTHeader parses a TPKT header from bytes.
func ParseTPKTHeader(data []byte) (*TPKTHeader, error) {
	if len(data) < TPKTHeaderSize {
		return nil, ErrS7InvalidLength
	}

	header := &TPKTHeader{
		Version:  data[0],
		Reserved: data[1],
		Length:   binary.BigEndian.Uint16(data[2:4]),
	}

	if header.Version != TPKTVersion {
		return nil, ErrS7InvalidTPKT
	}

	if header.Length < TPKTHeaderSize {
		return nil, ErrS7InvalidLength
	}

	return header, nil
}

// ParseCOTPHeader parses a COTP data TPDU header.
func ParseCOTPHeader(data []byte) (*COTPHeader, error) {
	if len(data) < COTPHeaderSize {
		return nil, ErrS7InvalidLength
	}

	header := &COTPHeader{
		Length:  data[0],
		PDUType: data[1],
		TPDUNR: data[2],
	}

	// For data transfer, PDU type should be 0xF0
	if header.PDUType != COTPDataTDPU &&
		header.PDUType != COTPConnectTDPU &&
		header.PDUType != COTPAcceptTDPU {
		return nil, ErrS7InvalidCOTP
	}

	return header, nil
}

// ParseS7Header parses an S7 communication header.
func ParseS7Header(data []byte) (*S7Header, error) {
	if len(data) < 10 {
		return nil, ErrS7InvalidLength
	}

	header := &S7Header{
		ProtocolID:   data[0],
		PDUType:      data[1],
		Reserved:     binary.BigEndian.Uint16(data[2:4]),
		PDUReference: binary.BigEndian.Uint16(data[4:6]),
		ParamLength:  binary.BigEndian.Uint16(data[6:8]),
		DataLength:   binary.BigEndian.Uint16(data[8:10]),
	}

	// Protocol ID must be 0x32
	if header.ProtocolID != 0x32 {
		return nil, ErrS7InvalidProtocol
	}

	// Validate PDU type
	switch header.PDUType {
	case S7PDUJobRequest, 0x02, S7PDUAckData, S7PDUUserData:
		// Valid
	default:
		return nil, ErrS7InvalidPDUType
	}

	// Validate param + data length doesn't exceed max PDU
	if int(header.ParamLength)+int(header.DataLength) > S7MaxPDUSize {
		return nil, ErrS7PDUTooLarge
	}

	return header, nil
}

// ParseS7Packet parses a complete S7 communication packet (TPKT + COTP + S7).
func ParseS7Packet(data []byte) (*TPKTHeader, *COTPHeader, *S7Header, error) {
	// Parse TPKT
	tpkt, err := ParseTPKTHeader(data)
	if err != nil {
		return nil, nil, nil, err
	}

	// Validate TPKT length against actual data
	if int(tpkt.Length) > len(data) {
		return nil, nil, nil, ErrS7InvalidLength
	}

	// Parse COTP (starts after TPKT header)
	if len(data) < TPKTHeaderSize+COTPHeaderSize {
		return nil, nil, nil, ErrS7InvalidLength
	}
	cotp, err := ParseCOTPHeader(data[TPKTHeaderSize:])
	if err != nil {
		return nil, nil, nil, err
	}

	// Parse S7 header (starts after TPKT + COTP)
	s7Start := TPKTHeaderSize + int(cotp.Length) + 1
	if s7Start >= len(data) {
		return nil, nil, nil, ErrS7InvalidLength
	}

	s7Header, err := ParseS7Header(data[s7Start:])
	if err != nil {
		return nil, nil, nil, err
	}

	return tpkt, cotp, s7Header, nil
}

// ValidateReadRequest validates an S7 read variable request.
func ValidateReadRequest(params []byte) error {
	if len(params) < 2 {
		return ErrS7InvalidLength
	}

	funcCode := params[0]
	if funcCode != S7FuncReadVar {
		return ErrS7InvalidFunction
	}

	itemCount := params[1]
	if itemCount == 0 {
		return ErrS7InvalidItemCount
	}
	if itemCount > S7MaxMultiReadItems {
		return ErrS7TooManyItems
	}

	// Each item is 12 bytes (S7Any addressing)
	expectedLen := 2 + int(itemCount)*12
	if len(params) < expectedLen {
		return ErrS7InvalidLength
	}

	// Validate each read item
	for i := 0; i < int(itemCount); i++ {
		offset := 2 + i*12
		item := params[offset : offset+12]
		if err := validateReadItem(item); err != nil {
			return err
		}
	}

	return nil
}

// validateReadItem validates a single S7 read item.
func validateReadItem(item []byte) error {
	if len(item) < 12 {
		return ErrS7InvalidLength
	}

	specType := item[0]
	if specType != 0x12 {
		return ErrS7InvalidSpecType
	}

	syntaxID := item[2]
	if syntaxID != 0x10 { // S7Any
		return ErrS7InvalidSyntax
	}

	// Validate transport size (word length)
	transportSize := item[3]
	switch transportSize {
	case S7WLBit, S7WLByte, S7WLWord, S7WLDWord, S7WLReal:
		// Valid
	default:
		return ErrS7InvalidWordLength
	}

	// Validate area code
	area := item[8]
	switch area {
	case S7AreaDB, S7AreaM, S7AreaI, S7AreaQ, S7AreaT, S7AreaC:
		// Valid
	default:
		return ErrS7InvalidArea
	}

	// DB number validation (only relevant for DB area)
	if area == S7AreaDB {
		dbNum := binary.BigEndian.Uint16(item[6:8])
		if dbNum == 0 {
			return ErrS7InvalidDBNumber
		}
	}

	return nil
}

// ValidateWriteRequest validates an S7 write variable request.
func ValidateWriteRequest(params []byte, writeData []byte) error {
	if len(params) < 2 {
		return ErrS7InvalidLength
	}

	funcCode := params[0]
	if funcCode != S7FuncWriteVar {
		return ErrS7InvalidFunction
	}

	itemCount := params[1]
	if itemCount == 0 {
		return ErrS7InvalidItemCount
	}
	if itemCount > S7MaxMultiReadItems {
		return ErrS7TooManyItems
	}

	return nil
}

// =============================================================================
// Error Types
// =============================================================================

type s7Error string

func (e s7Error) Error() string { return string(e) }

const (
	ErrS7InvalidLength    s7Error = "invalid packet length"
	ErrS7InvalidTPKT      s7Error = "invalid TPKT version"
	ErrS7InvalidCOTP      s7Error = "invalid COTP PDU type"
	ErrS7InvalidProtocol  s7Error = "invalid S7 protocol ID (expected 0x32)"
	ErrS7InvalidPDUType   s7Error = "invalid S7 PDU type"
	ErrS7PDUTooLarge      s7Error = "S7 PDU too large"
	ErrS7InvalidFunction  s7Error = "invalid S7 function code"
	ErrS7InvalidItemCount s7Error = "invalid item count"
	ErrS7TooManyItems     s7Error = "too many items (max 20)"
	ErrS7InvalidSpecType  s7Error = "invalid variable specification type"
	ErrS7InvalidSyntax    s7Error = "invalid syntax ID (expected S7Any 0x10)"
	ErrS7InvalidWordLength s7Error = "invalid word length code"
	ErrS7InvalidArea      s7Error = "invalid area code"
	ErrS7InvalidDBNumber  s7Error = "invalid DB number"
)

// =============================================================================
// Fuzz Tests
// =============================================================================

// FuzzS7PacketParsing tests full S7 packet parsing with random inputs.
func FuzzS7PacketParsing(f *testing.F) {
	// Valid S7 read request packet (TPKT + COTP + S7 header + read param)
	validReadPacket := []byte{
		// TPKT header
		0x03, 0x00, 0x00, 0x1F, // Version 3, length 31
		// COTP header
		0x02, 0xF0, 0x80, // Data TPDU, EOT
		// S7 header
		0x32,       // Protocol ID
		0x01,       // PDU type: Job
		0x00, 0x00, // Reserved
		0x00, 0x01, // PDU reference
		0x00, 0x0E, // Param length: 14
		0x00, 0x00, // Data length: 0
		// Read var parameters
		0x04, // Function: Read Var
		0x01, // Item count: 1
		// Item 1: Read DB1.DBW0
		0x12, 0x0A, 0x10, 0x04, // Spec, length, S7Any, Word
		0x00, 0x01, // Length: 1
		0x00, 0x01, // DB number: 1
		0x84,             // Area: DB
		0x00, 0x00, 0x00, // Address: 0
	}
	f.Add(validReadPacket)

	// Valid S7 setup communication
	validSetup := []byte{
		0x03, 0x00, 0x00, 0x19,
		0x02, 0xF0, 0x80,
		0x32, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x08, 0x00, 0x00,
		0xF0, 0x00, 0x00, 0x01, 0x00, 0x01, 0x01, 0xE0,
	}
	f.Add(validSetup)

	// Edge cases
	f.Add([]byte{})                           // Empty
	f.Add([]byte{0x03})                       // Partial TPKT
	f.Add([]byte{0x03, 0x00, 0x00, 0x04})    // TPKT only, no COTP
	f.Add([]byte{0x04, 0x00, 0x00, 0x10})    // Invalid TPKT version
	f.Add(bytes.Repeat([]byte{0xFF}, 500))    // Oversized
	f.Add(bytes.Repeat([]byte{0x00}, 30))     // All zeros

	f.Fuzz(func(t *testing.T, data []byte) {
		// Must not panic on any input
		tpkt, cotp, s7Header, err := ParseS7Packet(data)

		if err != nil {
			return
		}

		if tpkt == nil || cotp == nil || s7Header == nil {
			t.Error("ParseS7Packet returned nil without error")
			return
		}

		// If we got a valid S7 header with a read/write function, validate further
		s7Start := TPKTHeaderSize + int(cotp.Length) + 1
		remaining := data[s7Start+10:]
		if s7Header.PDUType == S7PDUJobRequest && len(remaining) > 0 {
			if remaining[0] == S7FuncReadVar {
				_ = ValidateReadRequest(remaining[:min(len(remaining), int(s7Header.ParamLength))])
			} else if remaining[0] == S7FuncWriteVar {
				_ = ValidateWriteRequest(remaining[:min(len(remaining), int(s7Header.ParamLength))], nil)
			}
		}
	})
}

// FuzzTPKTHeader tests TPKT header parsing with random inputs.
func FuzzTPKTHeader(f *testing.F) {
	f.Add([]byte{0x03, 0x00, 0x00, 0x1F})                // Valid
	f.Add([]byte{0x03, 0x00, 0x00, 0x04})                // Minimum length
	f.Add([]byte{0x03, 0x00, 0xFF, 0xFF})                // Max length
	f.Add([]byte{})                                        // Empty
	f.Add([]byte{0x04, 0x00, 0x00, 0x10})                // Wrong version
	f.Add([]byte{0x03, 0x00, 0x00, 0x02})                // Length too short

	f.Fuzz(func(t *testing.T, data []byte) {
		header, err := ParseTPKTHeader(data)
		if err != nil {
			return
		}
		if header == nil {
			t.Error("ParseTPKTHeader returned nil without error")
		}
	})
}

// FuzzS7Header tests S7 header parsing with random inputs.
func FuzzS7Header(f *testing.F) {
	// Valid job request header
	f.Add([]byte{0x32, 0x01, 0x00, 0x00, 0x00, 0x01, 0x00, 0x0E, 0x00, 0x00})
	// Valid ack-data header
	f.Add([]byte{0x32, 0x03, 0x00, 0x00, 0x00, 0x01, 0x00, 0x02, 0x00, 0x04})
	// Edge cases
	f.Add([]byte{})
	f.Add([]byte{0x32})
	f.Add([]byte{0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}) // Wrong protocol ID
	f.Add(bytes.Repeat([]byte{0x32}, 10)) // All 0x32

	f.Fuzz(func(t *testing.T, data []byte) {
		header, err := ParseS7Header(data)
		if err != nil {
			return
		}
		if header == nil {
			t.Error("ParseS7Header returned nil without error")
		}
		_ = header.PDUType
		_ = header.ParamLength
		_ = header.DataLength
	})
}

// FuzzS7ReadRequest tests S7 read variable request validation.
func FuzzS7ReadRequest(f *testing.F) {
	// Valid single item read (DB1.DBW0)
	validRead := []byte{
		0x04, 0x01, // Read var, 1 item
		0x12, 0x0A, 0x10, 0x04, // Spec, length, S7Any, Word
		0x00, 0x01, // Length: 1
		0x00, 0x01, // DB number: 1
		0x84,             // Area: DB
		0x00, 0x00, 0x00, // Address: 0
	}
	f.Add(validRead)

	// Multi-item read (2 items)
	multiRead := []byte{
		0x04, 0x02, // Read var, 2 items
		// Item 1: DB1.DBD0
		0x12, 0x0A, 0x10, 0x06, 0x00, 0x01, 0x00, 0x01, 0x84, 0x00, 0x00, 0x00,
		// Item 2: M area, MW10
		0x12, 0x0A, 0x10, 0x04, 0x00, 0x01, 0x00, 0x00, 0x83, 0x00, 0x00, 0x50,
	}
	f.Add(multiRead)

	// Edge cases
	f.Add([]byte{})
	f.Add([]byte{0x04, 0x00})         // Zero items
	f.Add([]byte{0x04, 0x15})         // 21 items (exceeds max)
	f.Add([]byte{0x05, 0x01})         // Wrong function code

	f.Fuzz(func(t *testing.T, data []byte) {
		_ = ValidateReadRequest(data)
	})
}

// FuzzS7ReadItem tests individual read item validation.
func FuzzS7ReadItem(f *testing.F) {
	// Valid DB read item
	f.Add([]byte{0x12, 0x0A, 0x10, 0x04, 0x00, 0x01, 0x00, 0x01, 0x84, 0x00, 0x00, 0x00})
	// Valid input area read
	f.Add([]byte{0x12, 0x0A, 0x10, 0x02, 0x00, 0x01, 0x00, 0x00, 0x81, 0x00, 0x00, 0x00})
	// Valid bit read
	f.Add([]byte{0x12, 0x0A, 0x10, 0x01, 0x00, 0x01, 0x00, 0x01, 0x84, 0x00, 0x00, 0x03})
	// Edge cases
	f.Add([]byte{})
	f.Add([]byte{0x12, 0x0A, 0x10, 0xFF, 0x00, 0x01, 0x00, 0x01, 0x84, 0x00, 0x00, 0x00}) // Bad word length
	f.Add([]byte{0x12, 0x0A, 0x10, 0x04, 0x00, 0x01, 0x00, 0x01, 0xFF, 0x00, 0x00, 0x00}) // Bad area

	f.Fuzz(func(t *testing.T, data []byte) {
		_ = validateReadItem(data)
	})
}

// FuzzS7FrameRoundTrip tests that valid packets can be parsed consistently.
func FuzzS7FrameRoundTrip(f *testing.F) {
	// Fuzz the inner S7 payload while keeping valid TPKT/COTP
	f.Add([]byte{0x01, 0x04, 0x01, 0x12, 0x0A, 0x10, 0x04, 0x00, 0x01, 0x00, 0x01, 0x84, 0x00, 0x00, 0x00})

	f.Fuzz(func(t *testing.T, payload []byte) {
		if len(payload) == 0 {
			return
		}

		// Wrap in valid TPKT + COTP + S7 header
		totalLen := TPKTHeaderSize + COTPHeaderSize + 10 + len(payload)
		if totalLen > 0xFFFF {
			return
		}

		packet := make([]byte, totalLen)
		// TPKT
		packet[0] = TPKTVersion
		packet[1] = TPKTReserved
		binary.BigEndian.PutUint16(packet[2:4], uint16(totalLen))
		// COTP
		packet[4] = 0x02
		packet[5] = COTPDataTDPU
		packet[6] = 0x80
		// S7 header
		packet[7] = 0x32 // Protocol ID
		packet[8] = S7PDUJobRequest
		binary.BigEndian.PutUint16(packet[9:11], 0)  // Reserved
		binary.BigEndian.PutUint16(packet[11:13], 1)  // PDU ref
		binary.BigEndian.PutUint16(packet[13:15], uint16(len(payload))) // Param length
		binary.BigEndian.PutUint16(packet[15:17], 0)  // Data length
		copy(packet[17:], payload)

		// Parse the constructed packet — must not panic
		tpkt, cotp, s7hdr, err := ParseS7Packet(packet)
		if err != nil {
			return
		}

		// Verify parsed values match what we constructed
		if tpkt.Length != uint16(totalLen) {
			t.Errorf("TPKT length mismatch: got %d, want %d", tpkt.Length, totalLen)
		}
		if cotp.PDUType != COTPDataTDPU {
			t.Errorf("COTP PDU type mismatch: got 0x%02X", cotp.PDUType)
		}
		if s7hdr.ParamLength != uint16(len(payload)) {
			t.Errorf("S7 param length mismatch: got %d, want %d", s7hdr.ParamLength, len(payload))
		}
	})
}
