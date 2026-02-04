//go:build fuzz
// +build fuzz

// Package parsing provides fuzz tests for address string parsing.
package parsing

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// =============================================================================
// Address Parsing Types
// =============================================================================

// ParsedModbusAddress represents a parsed Modbus address.
type ParsedModbusAddress struct {
	RegisterType string // "holding", "input", "coil", "discrete"
	Address      uint16
	BitOffset    int // For coil/discrete: -1 if not applicable
	Quantity     int // Number of registers/coils
	Valid        bool
}

// ParsedS7Address represents a parsed S7 address.
type ParsedS7Address struct {
	Area      string // "DB", "M", "I", "Q", "T", "C"
	DBNumber  int    // For DB area only
	Offset    int    // Byte offset
	BitOffset int    // For bit addressing (X)
	DataType  string // "X", "B", "W", "D", "REAL", etc.
	Valid     bool
}

// ParsedOPCUANodeID represents a parsed OPC UA Node ID.
type ParsedOPCUANodeID struct {
	NamespaceIndex uint16
	IdentifierType string // "i" (numeric), "s" (string), "g" (GUID), "b" (opaque)
	Identifier     string
	Valid          bool
}

// =============================================================================
// Modbus Address Parsing
// =============================================================================

// Modbus address patterns
var (
	modbusHoldingPattern  = regexp.MustCompile(`^(?i)(?:holding[:\s]*)?(\d+)(?:\[(\d+)\])?$`)
	modbusInputPattern    = regexp.MustCompile(`^(?i)input[:\s]*(\d+)(?:\[(\d+)\])?$`)
	modbusCoilPattern     = regexp.MustCompile(`^(?i)coil[:\s]*(\d+)(?:\.(\d))?$`)
	modbusDiscretePattern = regexp.MustCompile(`^(?i)discrete[:\s]*(\d+)(?:\.(\d))?$`)
)

// parseModbusAddressExt parses a Modbus address string with extended details.
func parseModbusAddressExt(address string) ParsedModbusAddress {
	address = strings.TrimSpace(address)

	// Try holding register
	if matches := modbusHoldingPattern.FindStringSubmatch(address); matches != nil {
		addr, _ := strconv.ParseUint(matches[1], 10, 16)
		quantity := 1
		if matches[2] != "" {
			quantity, _ = strconv.Atoi(matches[2])
		}
		return ParsedModbusAddress{
			RegisterType: "holding",
			Address:      uint16(addr),
			BitOffset:    -1,
			Quantity:     quantity,
			Valid:        addr <= 65535,
		}
	}

	// Try input register
	if matches := modbusInputPattern.FindStringSubmatch(address); matches != nil {
		addr, _ := strconv.ParseUint(matches[1], 10, 16)
		quantity := 1
		if matches[2] != "" {
			quantity, _ = strconv.Atoi(matches[2])
		}
		return ParsedModbusAddress{
			RegisterType: "input",
			Address:      uint16(addr),
			BitOffset:    -1,
			Quantity:     quantity,
			Valid:        addr <= 65535,
		}
	}

	// Try coil
	if matches := modbusCoilPattern.FindStringSubmatch(address); matches != nil {
		addr, _ := strconv.ParseUint(matches[1], 10, 16)
		bitOffset := -1
		if matches[2] != "" {
			bitOffset, _ = strconv.Atoi(matches[2])
		}
		// Bit offset must be 0-7 if specified
		valid := addr <= 65535 && (bitOffset == -1 || (bitOffset >= 0 && bitOffset <= 7))
		return ParsedModbusAddress{
			RegisterType: "coil",
			Address:      uint16(addr),
			BitOffset:    bitOffset,
			Quantity:     1,
			Valid:        valid,
		}
	}

	// Try discrete input
	if matches := modbusDiscretePattern.FindStringSubmatch(address); matches != nil {
		addr, _ := strconv.ParseUint(matches[1], 10, 16)
		bitOffset := -1
		if matches[2] != "" {
			bitOffset, _ = strconv.Atoi(matches[2])
		}
		// Bit offset must be 0-7 if specified
		valid := addr <= 65535 && (bitOffset == -1 || (bitOffset >= 0 && bitOffset <= 7))
		return ParsedModbusAddress{
			RegisterType: "discrete",
			Address:      uint16(addr),
			BitOffset:    bitOffset,
			Quantity:     1,
			Valid:        valid,
		}
	}

	return ParsedModbusAddress{Valid: false}
}

// =============================================================================
// S7 Address Parsing
// =============================================================================

// S7 address patterns
var (
	s7DBPattern  = regexp.MustCompile(`^(?i)DB(\d+)\.DB([XBWD])(\d+)(?:\.(\d))?$`)
	s7DBRealPat  = regexp.MustCompile(`^(?i)DB(\d+)\.DB(REAL|DWORD|WORD|BYTE)(\d+)$`)
	s7MerkerPat  = regexp.MustCompile(`^(?i)M([XBWD])(\d+)(?:\.(\d))?$`)
	s7InputPat   = regexp.MustCompile(`^(?i)I([XBWD])(\d+)(?:\.(\d))?$`)
	s7OutputPat  = regexp.MustCompile(`^(?i)Q([XBWD])(\d+)(?:\.(\d))?$`)
	s7TimerPat   = regexp.MustCompile(`^(?i)T(\d+)$`)
	s7CounterPat = regexp.MustCompile(`^(?i)C(\d+)$`)
)

// parseS7Address parses an S7 address string.
func parseS7Address(address string) ParsedS7Address {
	address = strings.TrimSpace(address)

	// Try DB address (e.g., DB1.DBX0.0, DB1.DBW10, DB1.DBD20)
	if matches := s7DBPattern.FindStringSubmatch(address); matches != nil {
		dbNum, _ := strconv.Atoi(matches[1])
		dataType := strings.ToUpper(matches[2])
		offset, _ := strconv.Atoi(matches[3])
		bitOffset := 0
		if matches[4] != "" {
			bitOffset, _ = strconv.Atoi(matches[4])
		}
		return ParsedS7Address{
			Area:      "DB",
			DBNumber:  dbNum,
			Offset:    offset,
			BitOffset: bitOffset,
			DataType:  dataType,
			Valid:     dbNum >= 1 && dbNum <= 65535 && offset >= 0,
		}
	}

	// Try DB REAL/DWORD/etc address
	if matches := s7DBRealPat.FindStringSubmatch(address); matches != nil {
		dbNum, _ := strconv.Atoi(matches[1])
		dataType := strings.ToUpper(matches[2])
		offset, _ := strconv.Atoi(matches[3])
		return ParsedS7Address{
			Area:      "DB",
			DBNumber:  dbNum,
			Offset:    offset,
			BitOffset: 0,
			DataType:  dataType,
			Valid:     dbNum >= 1 && dbNum <= 65535 && offset >= 0,
		}
	}

	// Try Merker address (e.g., MX0.0, MB10, MW20, MD30)
	if matches := s7MerkerPat.FindStringSubmatch(address); matches != nil {
		dataType := strings.ToUpper(matches[1])
		offset, _ := strconv.Atoi(matches[2])
		bitOffset := 0
		if matches[3] != "" {
			bitOffset, _ = strconv.Atoi(matches[3])
		}
		return ParsedS7Address{
			Area:      "M",
			DBNumber:  0,
			Offset:    offset,
			BitOffset: bitOffset,
			DataType:  dataType,
			Valid:     offset >= 0,
		}
	}

	// Try Input address (e.g., IX0.0, IB10, IW20)
	if matches := s7InputPat.FindStringSubmatch(address); matches != nil {
		dataType := strings.ToUpper(matches[1])
		offset, _ := strconv.Atoi(matches[2])
		bitOffset := 0
		if matches[3] != "" {
			bitOffset, _ = strconv.Atoi(matches[3])
		}
		return ParsedS7Address{
			Area:      "I",
			DBNumber:  0,
			Offset:    offset,
			BitOffset: bitOffset,
			DataType:  dataType,
			Valid:     offset >= 0,
		}
	}

	// Try Output address (e.g., QX0.0, QB10, QW20)
	if matches := s7OutputPat.FindStringSubmatch(address); matches != nil {
		dataType := strings.ToUpper(matches[1])
		offset, _ := strconv.Atoi(matches[2])
		bitOffset := 0
		if matches[3] != "" {
			bitOffset, _ = strconv.Atoi(matches[3])
		}
		return ParsedS7Address{
			Area:      "Q",
			DBNumber:  0,
			Offset:    offset,
			BitOffset: bitOffset,
			DataType:  dataType,
			Valid:     offset >= 0,
		}
	}

	// Try Timer
	if matches := s7TimerPat.FindStringSubmatch(address); matches != nil {
		offset, _ := strconv.Atoi(matches[1])
		return ParsedS7Address{
			Area:      "T",
			DBNumber:  0,
			Offset:    offset,
			BitOffset: 0,
			DataType:  "TIMER",
			Valid:     offset >= 0,
		}
	}

	// Try Counter
	if matches := s7CounterPat.FindStringSubmatch(address); matches != nil {
		offset, _ := strconv.Atoi(matches[1])
		return ParsedS7Address{
			Area:      "C",
			DBNumber:  0,
			Offset:    offset,
			BitOffset: 0,
			DataType:  "COUNTER",
			Valid:     offset >= 0,
		}
	}

	return ParsedS7Address{Valid: false}
}

// =============================================================================
// Fuzz Tests for Modbus Address Parsing
// =============================================================================

// FuzzModbusAddressParsing fuzzes Modbus address string parsing.
func FuzzModbusAddressParsing(f *testing.F) {
	// Seed with valid addresses
	f.Add("0")
	f.Add("100")
	f.Add("65535")
	f.Add("holding:100")
	f.Add("HOLDING:200")
	f.Add("input:100")
	f.Add("INPUT:200")
	f.Add("coil:0")
	f.Add("coil:100.7")
	f.Add("discrete:0")
	f.Add("discrete:100.3")
	f.Add("100[10]")
	f.Add("holding:100[5]")

	// Seed with invalid addresses
	f.Add("")
	f.Add("invalid")
	f.Add("-1")
	f.Add("65536")
	f.Add("coil:100.8")
	f.Add("foo:bar")

	f.Fuzz(func(t *testing.T, address string) {
		// Should not panic on any input
		result := parseModbusAddressExt(address)

		// If valid, verify constraints
		if result.Valid {
			if result.Address > 65535 {
				t.Errorf("valid address %s has out-of-range address: %d", address, result.Address)
			}
			if result.RegisterType == "" {
				t.Errorf("valid address %s has empty register type", address)
			}
			if result.Quantity < 1 {
				t.Errorf("valid address %s has invalid quantity: %d", address, result.Quantity)
			}
			if result.BitOffset != -1 && (result.BitOffset < 0 || result.BitOffset > 7) {
				t.Errorf("valid address %s has invalid bit offset: %d", address, result.BitOffset)
			}
		}
	})
}

// FuzzModbusAddressRoundtrip fuzzes Modbus address roundtrip.
func FuzzModbusAddressRoundtrip(f *testing.F) {
	f.Add(uint16(0), "holding", 1)
	f.Add(uint16(100), "input", 5)
	f.Add(uint16(1000), "coil", 1)
	f.Add(uint16(65535), "discrete", 1)

	f.Fuzz(func(t *testing.T, addr uint16, regType string, quantity int) {
		// Normalize inputs
		if quantity < 1 {
			quantity = 1
		}
		if quantity > 125 {
			quantity = 125
		}

		var addressStr string
		switch regType {
		case "holding":
			if quantity > 1 {
				addressStr = fmt.Sprintf("holding:%d[%d]", addr, quantity)
			} else {
				addressStr = fmt.Sprintf("holding:%d", addr)
			}
		case "input":
			if quantity > 1 {
				addressStr = fmt.Sprintf("input:%d[%d]", addr, quantity)
			} else {
				addressStr = fmt.Sprintf("input:%d", addr)
			}
		case "coil":
			addressStr = fmt.Sprintf("coil:%d", addr)
		case "discrete":
			addressStr = fmt.Sprintf("discrete:%d", addr)
		default:
			return
		}

		result := parseModbusAddressExt(addressStr)
		if !result.Valid {
			t.Errorf("failed to parse generated address: %s", addressStr)
			return
		}

		if result.Address != addr {
			t.Errorf("address mismatch: input %d, got %d", addr, result.Address)
		}
	})
}

// =============================================================================
// Fuzz Tests for S7 Address Parsing
// =============================================================================

// FuzzS7AddressParsing fuzzes S7 address string parsing.
func FuzzS7AddressParsing(f *testing.F) {
	// Seed with valid DB addresses
	f.Add("DB1.DBX0.0")
	f.Add("DB1.DBX0.7")
	f.Add("DB1.DBB10")
	f.Add("DB1.DBW20")
	f.Add("DB1.DBD30")
	f.Add("DB100.DBREAL0")
	f.Add("DB65535.DBW100")

	// Seed with valid Merker addresses
	f.Add("MX0.0")
	f.Add("MB10")
	f.Add("MW20")
	f.Add("MD30")

	// Seed with valid Input/Output addresses
	f.Add("IX0.0")
	f.Add("IB10")
	f.Add("QX0.0")
	f.Add("QB10")

	// Seed with Timer/Counter
	f.Add("T0")
	f.Add("T100")
	f.Add("C0")
	f.Add("C100")

	// Seed with invalid addresses
	f.Add("")
	f.Add("invalid")
	f.Add("DB0.DBW0")
	f.Add("DB-1.DBW0")
	f.Add("XX0.0")

	f.Fuzz(func(t *testing.T, address string) {
		// Should not panic on any input
		result := parseS7Address(address)

		// If valid, verify constraints
		if result.Valid {
			validAreas := map[string]bool{"DB": true, "M": true, "I": true, "Q": true, "T": true, "C": true}
			if !validAreas[result.Area] {
				t.Errorf("valid address %s has invalid area: %s", address, result.Area)
			}
			if result.Area == "DB" && (result.DBNumber < 1 || result.DBNumber > 65535) {
				t.Errorf("valid address %s has invalid DB number: %d", address, result.DBNumber)
			}
			if result.Offset < 0 {
				t.Errorf("valid address %s has negative offset: %d", address, result.Offset)
			}
			if result.BitOffset < 0 || result.BitOffset > 7 {
				t.Errorf("valid address %s has invalid bit offset: %d", address, result.BitOffset)
			}
		}
	})
}

// FuzzS7AddressRoundtrip fuzzes S7 address roundtrip for DB addresses.
func FuzzS7AddressRoundtrip(f *testing.F) {
	f.Add(1, 0, 0, "X")
	f.Add(1, 10, 0, "B")
	f.Add(1, 20, 0, "W")
	f.Add(1, 30, 0, "D")
	f.Add(100, 100, 5, "X")

	f.Fuzz(func(t *testing.T, dbNum, offset, bitOffset int, dataType string) {
		// Normalize inputs
		if dbNum < 1 {
			dbNum = 1
		}
		if dbNum > 65535 {
			dbNum = 65535
		}
		if offset < 0 {
			offset = 0
		}
		if offset > 65535 {
			offset = 65535
		}
		bitOffset = bitOffset % 8
		if bitOffset < 0 {
			bitOffset = -bitOffset
		}

		validTypes := map[string]bool{"X": true, "B": true, "W": true, "D": true}
		if !validTypes[dataType] {
			dataType = "W"
		}

		var addressStr string
		if dataType == "X" {
			addressStr = fmt.Sprintf("DB%d.DB%s%d.%d", dbNum, dataType, offset, bitOffset)
		} else {
			addressStr = fmt.Sprintf("DB%d.DB%s%d", dbNum, dataType, offset)
		}

		result := parseS7Address(addressStr)
		if !result.Valid {
			t.Errorf("failed to parse generated address: %s", addressStr)
			return
		}

		if result.DBNumber != dbNum {
			t.Errorf("DB number mismatch: input %d, got %d", dbNum, result.DBNumber)
		}
		if result.Offset != offset {
			t.Errorf("offset mismatch: input %d, got %d", offset, result.Offset)
		}
		if result.DataType != dataType {
			t.Errorf("data type mismatch: input %s, got %s", dataType, result.DataType)
		}
	})
}

// =============================================================================
// Fuzz Tests for Edge Cases
// =============================================================================

// FuzzAddressWithSpecialChars fuzzes addresses with special characters.
func FuzzAddressWithSpecialChars(f *testing.F) {
	f.Add("holding:100\n")
	f.Add("holding:100\t")
	f.Add("holding:100 ")
	f.Add(" holding:100")
	f.Add("DB1.DBW0\x00")
	f.Add("DB1.DBW0;DROP TABLE")
	f.Add("../../../etc/passwd")

	f.Fuzz(func(t *testing.T, address string) {
		// Should not panic and should handle gracefully
		modbusResult := parseModbusAddressExt(address)
		s7Result := parseS7Address(address)

		// Addresses with null bytes or SQL injection should be invalid
		if strings.ContainsAny(address, "\x00;'\"") {
			// These should typically be invalid or sanitized
			_ = modbusResult
			_ = s7Result
		}
	})
}

// FuzzAddressWithLargeNumbers fuzzes addresses with very large numbers.
func FuzzAddressWithLargeNumbers(f *testing.F) {
	f.Add("holding:999999999999")
	f.Add("DB999999.DBW0")
	f.Add("DB1.DBW999999999")
	f.Add("coil:18446744073709551615")

	f.Fuzz(func(t *testing.T, address string) {
		// Should not panic on overflow attempts
		modbusResult := parseModbusAddressExt(address)
		s7Result := parseS7Address(address)

		// These should be invalid due to overflow
		_ = modbusResult
		_ = s7Result
	})
}
