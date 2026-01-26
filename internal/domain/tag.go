// Package domain contains core business entities.
package domain

import (
	"fmt"
	"time"
)

// DataType represents the data type of a tag value.
type DataType string

const (
	DataTypeBool    DataType = "bool"
	DataTypeInt16   DataType = "int16"
	DataTypeUInt16  DataType = "uint16"
	DataTypeInt32   DataType = "int32"
	DataTypeUInt32  DataType = "uint32"
	DataTypeInt64   DataType = "int64"
	DataTypeUInt64  DataType = "uint64"
	DataTypeFloat32 DataType = "float32"
	DataTypeFloat64 DataType = "float64"
	DataTypeString  DataType = "string"
)

// RegisterType represents the Modbus register type.
type RegisterType string

const (
	RegisterTypeCoil            RegisterType = "coil"             // Read/Write, 1 bit
	RegisterTypeDiscreteInput   RegisterType = "discrete_input"   // Read-only, 1 bit
	RegisterTypeHoldingRegister RegisterType = "holding_register" // Read/Write, 16 bits
	RegisterTypeInputRegister   RegisterType = "input_register"   // Read-only, 16 bits
)

// ByteOrder represents the byte ordering for multi-byte values.
type ByteOrder string

const (
	ByteOrderBigEndian    ByteOrder = "big_endian"    // ABCD (most common)
	ByteOrderLittleEndian ByteOrder = "little_endian" // DCBA
	ByteOrderMidBigEndian ByteOrder = "mid_big"       // BADC (word swap)
	ByteOrderMidLitEndian ByteOrder = "mid_little"    // CDAB (byte swap)
)

// AccessMode defines read/write access for a tag.
type AccessMode string

const (
	AccessModeReadOnly  AccessMode = "read"
	AccessModeWriteOnly AccessMode = "write"
	AccessModeReadWrite AccessMode = "readwrite"
)

// Tag represents a single data point to be read from/written to a device.
type Tag struct {
	// ID is the unique identifier for this tag within the device
	ID string `json:"id" yaml:"id"`

	// Name is a human-readable name for the tag
	Name string `json:"name" yaml:"name"`

	// Description provides additional context about the tag
	Description string `json:"description,omitempty" yaml:"description,omitempty"`

	// Address is the protocol-specific address (e.g., Modbus register address)
	Address uint16 `json:"address" yaml:"address"`

	// RegisterType specifies the Modbus register type
	RegisterType RegisterType `json:"register_type" yaml:"register_type"`

	// DataType specifies how to interpret the raw data
	DataType DataType `json:"data_type" yaml:"data_type"`

	// ByteOrder specifies byte ordering for multi-byte values
	ByteOrder ByteOrder `json:"byte_order,omitempty" yaml:"byte_order,omitempty"`

	// RegisterCount is the number of registers to read (for multi-register values)
	RegisterCount uint16 `json:"register_count,omitempty" yaml:"register_count,omitempty"`

	// BitPosition is the bit position for boolean values within a register (0-15)
	BitPosition *uint8 `json:"bit_position,omitempty" yaml:"bit_position,omitempty"`

	// ScaleFactor is multiplied with the raw value to get the engineering value
	ScaleFactor float64 `json:"scale_factor,omitempty" yaml:"scale_factor,omitempty"`

	// Offset is added to the scaled value
	Offset float64 `json:"offset,omitempty" yaml:"offset,omitempty"`

	// Unit is the engineering unit (e.g., "°C", "bar", "m/s")
	Unit string `json:"unit,omitempty" yaml:"unit,omitempty"`

	// TopicSuffix is appended to the device's UNS prefix to form the MQTT topic
	// e.g., if UNS prefix is "plant1/line1/plc1" and suffix is "temperature"
	// the full topic would be "plant1/line1/plc1/temperature"
	TopicSuffix string `json:"topic_suffix" yaml:"topic_suffix"`

	// PollInterval overrides the device's default poll interval for this tag
	PollInterval *time.Duration `json:"poll_interval,omitempty" yaml:"poll_interval,omitempty"`

	// DeadbandType specifies how deadband filtering is applied
	DeadbandType DeadbandType `json:"deadband_type,omitempty" yaml:"deadband_type,omitempty"`

	// DeadbandValue is the threshold for deadband filtering
	DeadbandValue float64 `json:"deadband_value,omitempty" yaml:"deadband_value,omitempty"`

	// Enabled indicates whether this tag should be actively polled
	Enabled bool `json:"enabled" yaml:"enabled"`

	// AccessMode specifies read/write access (read, write, readwrite)
	AccessMode AccessMode `json:"access_mode,omitempty" yaml:"access_mode,omitempty"`

	// OPCNodeID is the OPC UA node identifier (e.g., "ns=2;s=Temperature" or "ns=2;i=1234")
	OPCNodeID string `json:"opc_node_id,omitempty" yaml:"opc_node_id,omitempty"`

	// OPCNamespaceIndex is the OPC UA namespace index (if not included in OPCNodeID)
	OPCNamespaceIndex uint16 `json:"opc_namespace_index,omitempty" yaml:"opc_namespace_index,omitempty"`

	// === S7 (Siemens) Specific Fields ===

	// S7Area specifies the memory area (DB, M, I, Q, T, C)
	S7Area S7Area `json:"s7_area,omitempty" yaml:"s7_area,omitempty"`

	// S7DBNumber is the data block number (only for S7Area = DB)
	S7DBNumber int `json:"s7_db_number,omitempty" yaml:"s7_db_number,omitempty"`

	// S7Offset is the byte offset within the memory area
	S7Offset int `json:"s7_offset,omitempty" yaml:"s7_offset,omitempty"`

	// S7BitOffset is the bit offset for boolean types (0-7)
	S7BitOffset int `json:"s7_bit_offset,omitempty" yaml:"s7_bit_offset,omitempty"`

	// S7Address is a symbolic address string (e.g., "DB1.DBD0", "MW100", "I0.0")
	// If provided, this will be parsed to extract Area, DBNumber, Offset, and BitOffset
	S7Address string `json:"s7_address,omitempty" yaml:"s7_address,omitempty"`

	// Metadata contains additional key-value pairs for this tag
	Metadata map[string]string `json:"metadata,omitempty" yaml:"metadata,omitempty"`
}

// S7Area represents the Siemens S7 memory area type.
type S7Area string

const (
	S7AreaDB S7Area = "DB" // Data Block
	S7AreaM  S7Area = "M"  // Merker/Flags
	S7AreaI  S7Area = "I"  // Inputs
	S7AreaQ  S7Area = "Q"  // Outputs
	S7AreaT  S7Area = "T"  // Timers
	S7AreaC  S7Area = "C"  // Counters
)

// DeadbandType specifies how deadband filtering is applied.
type DeadbandType string

const (
	DeadbandTypeNone     DeadbandType = "none"     // No deadband filtering
	DeadbandTypeAbsolute DeadbandType = "absolute" // Absolute value change threshold
	DeadbandTypePercent  DeadbandType = "percent"  // Percentage change threshold
)

// Validate performs validation on the tag configuration.
func (t *Tag) Validate() error {
	if t.ID == "" {
		return fmt.Errorf("tag ID is required")
	}
	if t.Name == "" {
		return fmt.Errorf("tag name is required")
	}
	if t.TopicSuffix == "" {
		return fmt.Errorf("tag topic suffix is required")
	}
	if t.DataType == "" {
		return fmt.Errorf("data type is required for tag %s", t.ID)
	}

	// RegisterType is only required for Modbus tags (not OPC UA or S7)
	// OPC UA uses OPCNodeID, S7 uses S7Address
	isModbus := t.OPCNodeID == "" && t.S7Address == ""
	if isModbus && t.RegisterType == "" {
		return fmt.Errorf("register type is required for Modbus tag %s", t.ID)
	}

	// Validate register count based on data type (Modbus only)
	if isModbus {
		expectedCount := t.ExpectedRegisterCount()
		if t.RegisterCount == 0 {
			t.RegisterCount = expectedCount
		} else if t.RegisterCount < expectedCount {
			return fmt.Errorf("register count %d is insufficient for data type %s (needs %d)",
				t.RegisterCount, t.DataType, expectedCount)
		}

		// Set default byte order
		if t.ByteOrder == "" {
			t.ByteOrder = ByteOrderBigEndian
		}
	}

	// Set default scale factor
	if t.ScaleFactor == 0 {
		t.ScaleFactor = 1.0
	}

	return nil
}

// ExpectedRegisterCount returns the number of 16-bit registers needed for the data type.
func (t *Tag) ExpectedRegisterCount() uint16 {
	switch t.DataType {
	case DataTypeBool, DataTypeInt16, DataTypeUInt16:
		return 1
	case DataTypeInt32, DataTypeUInt32, DataTypeFloat32:
		return 2
	case DataTypeInt64, DataTypeUInt64, DataTypeFloat64:
		return 4
	default:
		return 1
	}
}

// GetEffectivePollInterval returns the effective poll interval for this tag.
func (t *Tag) GetEffectivePollInterval(deviceDefault time.Duration) time.Duration {
	if t.PollInterval != nil {
		return *t.PollInterval
	}
	return deviceDefault
}

// IsWritable returns true if the tag supports write operations.
// For Modbus, coils and holding registers are writable.
// For OPC UA, it depends on the node's access level (use AccessMode).
func (t *Tag) IsWritable() bool {
	// Check explicit access mode first
	if t.AccessMode != "" {
		return t.AccessMode == AccessModeWriteOnly || t.AccessMode == AccessModeReadWrite
	}

	// For Modbus, determine writability from register type
	switch t.RegisterType {
	case RegisterTypeCoil:
		return true // Coils are read/write
	case RegisterTypeHoldingRegister:
		return true // Holding registers are read/write
	case RegisterTypeDiscreteInput:
		return false // Discrete inputs are read-only
	case RegisterTypeInputRegister:
		return false // Input registers are read-only
	default:
		// For OPC UA and other protocols, assume writable if access mode is set
		return false
	}
}

// IsReadable returns true if the tag supports read operations.
func (t *Tag) IsReadable() bool {
	if t.AccessMode != "" {
		return t.AccessMode == AccessModeReadOnly || t.AccessMode == AccessModeReadWrite
	}
	// By default, all tags are readable
	return true
}

