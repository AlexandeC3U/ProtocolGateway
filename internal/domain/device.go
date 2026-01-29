// Package domain contains the core business entities and interfaces.
// These are protocol-agnostic and represent the core concepts of the system.
package domain

import (
	"fmt"
	"time"
)

// DeviceStatus represents the current operational status of a device.
type DeviceStatus string

const (
	DeviceStatusOnline     DeviceStatus = "online"
	DeviceStatusOffline    DeviceStatus = "offline"
	DeviceStatusConnecting DeviceStatus = "connecting"
	DeviceStatusError      DeviceStatus = "error"
	DeviceStatusUnknown    DeviceStatus = "unknown"
)

// Protocol represents the communication protocol type.
type Protocol string

const (
	ProtocolModbusTCP Protocol = "modbus-tcp"
	ProtocolModbusRTU Protocol = "modbus-rtu"
	ProtocolOPCUA     Protocol = "opcua"
	ProtocolS7        Protocol = "s7"
	ProtocolMQTT      Protocol = "mqtt"
)

// Device represents a connected industrial device.
type Device struct {
	// ID is the unique identifier for this device
	ID string `json:"id" yaml:"id"`

	// Name is a human-readable name for the device
	Name string `json:"name" yaml:"name"`

	// Description provides additional context about the device
	Description string `json:"description,omitempty" yaml:"description,omitempty"`

	// Protocol specifies the communication protocol
	Protocol Protocol `json:"protocol" yaml:"protocol"`

	// Connection holds protocol-specific connection parameters
	Connection ConnectionConfig `json:"connection" yaml:"connection"`

	// Tags defines the data points to be collected from this device
	Tags []Tag `json:"tags" yaml:"tags"`

	// PollInterval is the default polling interval for all tags (can be overridden per tag)
	PollInterval time.Duration `json:"poll_interval" yaml:"poll_interval"`

	// Enabled indicates whether this device should be actively polled
	Enabled bool `json:"enabled" yaml:"enabled"`

	// UNSPrefix is the Unified Namespace prefix for this device's topics
	// e.g., "plant1/area2/line3/device1"
	UNSPrefix string `json:"uns_prefix" yaml:"uns_prefix"`

	// Metadata contains additional key-value pairs for this device
	Metadata map[string]string `json:"metadata,omitempty" yaml:"metadata,omitempty"`

	// CreatedAt is when this device configuration was created
	CreatedAt time.Time `json:"created_at" yaml:"created_at"`

	// UpdatedAt is when this device configuration was last modified
	UpdatedAt time.Time `json:"updated_at" yaml:"updated_at"`

	// ConfigVersion tracks configuration revisions for fleet management.
	// Incremented on each configuration update.
	ConfigVersion uint32 `json:"config_version,omitempty" yaml:"config_version,omitempty"`

	// ActiveConfigVersion is the currently running configuration version.
	// May differ from ConfigVersion during rolling updates.
	ActiveConfigVersion uint32 `json:"active_config_version,omitempty" yaml:"active_config_version,omitempty"`

	// LastKnownGoodVersion is the last configuration that worked successfully.
	// Used for automatic rollback on failures.
	LastKnownGoodVersion uint32 `json:"last_known_good_version,omitempty" yaml:"last_known_good_version,omitempty"`
}

// ConnectionConfig holds protocol-specific connection parameters.
type ConnectionConfig struct {
	// === Common Settings ===

	// Host is the IP address or hostname of the device
	Host string `json:"host,omitempty" yaml:"host,omitempty"`

	// Port is the TCP port number
	Port int `json:"port,omitempty" yaml:"port,omitempty"`

	// Timeout is the connection/response timeout
	Timeout time.Duration `json:"timeout,omitempty" yaml:"timeout,omitempty"`

	// RetryCount is the number of retry attempts on failure
	RetryCount int `json:"retry_count,omitempty" yaml:"retry_count,omitempty"`

	// RetryDelay is the delay between retry attempts
	RetryDelay time.Duration `json:"retry_delay,omitempty" yaml:"retry_delay,omitempty"`

	// === Modbus Settings ===

	// SlaveID is the Modbus slave/unit ID (1-247)
	SlaveID uint8 `json:"slave_id,omitempty" yaml:"slave_id,omitempty"`

	// SerialPort is the serial port path for RTU connections (e.g., "/dev/ttyUSB0")
	SerialPort string `json:"serial_port,omitempty" yaml:"serial_port,omitempty"`

	// BaudRate is the serial baud rate for RTU connections
	BaudRate int `json:"baud_rate,omitempty" yaml:"baud_rate,omitempty"`

	// DataBits is the number of data bits for RTU connections (5, 6, 7, or 8)
	DataBits int `json:"data_bits,omitempty" yaml:"data_bits,omitempty"`

	// Parity is the parity setting for RTU connections ("N", "E", "O")
	Parity string `json:"parity,omitempty" yaml:"parity,omitempty"`

	// StopBits is the number of stop bits for RTU connections (1 or 2)
	StopBits int `json:"stop_bits,omitempty" yaml:"stop_bits,omitempty"`

	// === OPC UA Settings ===

	// OPCEndpointURL is the full OPC UA endpoint URL (e.g., "opc.tcp://localhost:4840")
	// If not provided, it will be constructed from Host:Port
	OPCEndpointURL string `json:"opc_endpoint_url,omitempty" yaml:"opc_endpoint_url,omitempty"`

	// OPCSecurityPolicy specifies the security policy (None, Basic128Rsa15, Basic256, Basic256Sha256)
	OPCSecurityPolicy string `json:"opc_security_policy,omitempty" yaml:"opc_security_policy,omitempty"`

	// OPCSecurityMode specifies the security mode (None, Sign, SignAndEncrypt)
	OPCSecurityMode string `json:"opc_security_mode,omitempty" yaml:"opc_security_mode,omitempty"`

	// OPCAuthMode specifies authentication mode (Anonymous, UserName, Certificate)
	OPCAuthMode string `json:"opc_auth_mode,omitempty" yaml:"opc_auth_mode,omitempty"`

	// OPCUsername for UserName authentication
	OPCUsername string `json:"opc_username,omitempty" yaml:"opc_username,omitempty"`

	// OPCPassword for UserName authentication
	OPCPassword string `json:"opc_password,omitempty" yaml:"opc_password,omitempty"`

	// OPCCertFile path for certificate authentication
	OPCCertFile string `json:"opc_cert_file,omitempty" yaml:"opc_cert_file,omitempty"`

	// OPCKeyFile path for private key (certificate authentication)
	OPCKeyFile string `json:"opc_key_file,omitempty" yaml:"opc_key_file,omitempty"`

	// OPCPublishInterval is the subscription publish interval for OPC UA
	OPCPublishInterval time.Duration `json:"opc_publish_interval,omitempty" yaml:"opc_publish_interval,omitempty"`

	// OPCSamplingInterval is the sampling interval for OPC UA monitored items
	OPCSamplingInterval time.Duration `json:"opc_sampling_interval,omitempty" yaml:"opc_sampling_interval,omitempty"`

	// OPCUseSubscriptions enables OPC UA subscriptions (Report-by-Exception) instead of polling.
	// When true, the server pushes data changes to the client, which is more efficient
	// for slow-changing values. Requires OPCPublishInterval and OPCSamplingInterval.
	// NOTE: Not yet implemented - planned for Phase 3 (Gateway Core).
	// Currently all OPC UA devices use polling.
	OPCUseSubscriptions bool `json:"opc_use_subscriptions,omitempty" yaml:"opc_use_subscriptions,omitempty"`

	// === S7 (Siemens) Settings ===

	// S7Rack is the rack number of the PLC (usually 0)
	S7Rack int `json:"s7_rack,omitempty" yaml:"s7_rack,omitempty"`

	// S7Slot is the slot number of the CPU (usually 1 for S7-300/400, 0 or 1 for S7-1200/1500)
	S7Slot int `json:"s7_slot,omitempty" yaml:"s7_slot,omitempty"`

	// S7PDUSize is the maximum PDU size for communication (default: 480)
	S7PDUSize int `json:"s7_pdu_size,omitempty" yaml:"s7_pdu_size,omitempty"`

	// S7Timeout is the connection timeout for S7 (default: 10s)
	S7Timeout time.Duration `json:"s7_timeout,omitempty" yaml:"s7_timeout,omitempty"`
}

// Validate performs validation on the device configuration.
func (d *Device) Validate() error {
	if d.ID == "" {
		return ErrDeviceIDRequired
	}
	if d.Name == "" {
		return ErrDeviceNameRequired
	}
	if d.Protocol == "" {
		return ErrProtocolRequired
	}
	if len(d.Tags) == 0 {
		return ErrNoTagsDefined
	}
	if d.PollInterval < time.Millisecond*100 {
		return ErrPollIntervalTooShort
	}
	if d.UNSPrefix == "" {
		return ErrUNSPrefixRequired
	}

	for i := range d.Tags {
		if err := d.Tags[i].ValidateForProtocol(d.Protocol); err != nil {
			return fmt.Errorf("invalid tag %q for device %q: %w", d.Tags[i].ID, d.ID, err)
		}
	}
	return nil
}

// GetAddress returns the full address string for this device.
func (d *Device) GetAddress() string {
	if d.Connection.Host != "" {
		return d.Connection.Host
	}
	return d.Connection.SerialPort
}
