// Package config provides device configuration loading and management.
package config

import (
	"fmt"
	"os"
	"time"

	"github.com/nexus-edge/protocol-gateway/internal/domain"
	"gopkg.in/yaml.v3"
)

// DeviceConfig represents the YAML structure for device configuration.
type DeviceConfig struct {
	ID           string            `yaml:"id"`
	Name         string            `yaml:"name"`
	Description  string            `yaml:"description,omitempty"`
	Protocol     string            `yaml:"protocol"`
	Enabled      bool              `yaml:"enabled"`
	UNSPrefix    string            `yaml:"uns_prefix"`
	PollInterval string            `yaml:"poll_interval,omitempty"`
	Connection   ConnectionConfig  `yaml:"connection"`
	Tags         []TagConfig       `yaml:"tags"`
	Metadata     map[string]string `yaml:"metadata,omitempty"`
}

// ConnectionConfig represents connection settings in YAML.
type ConnectionConfig struct {
	// Common
	Host       string `yaml:"host"`
	Port       int    `yaml:"port"`
	Timeout    string `yaml:"timeout"`
	RetryCount int    `yaml:"retry_count"`
	RetryDelay string `yaml:"retry_delay"`

	// Modbus
	SlaveID int `yaml:"slave_id"`

	// OPC UA
	OPCEndpointURL        string `yaml:"opc_endpoint_url"`
	OPCSecurityPolicy     string `yaml:"opc_security_policy"`
	OPCSecurityMode       string `yaml:"opc_security_mode"`
	OPCAuthMode           string `yaml:"opc_auth_mode"`
	OPCUsername           string `yaml:"opc_username"`
	OPCPassword           string `yaml:"opc_password"`
	OPCCertFile           string `yaml:"opc_cert_file"`
	OPCKeyFile            string `yaml:"opc_key_file"`
	OPCServerCertFile     string `yaml:"opc_server_cert_file"`
	OPCInsecureSkipVerify bool   `yaml:"opc_insecure_skip_verify"`
	OPCAutoSelectEndpoint bool   `yaml:"opc_auto_select_endpoint"`
	OPCApplicationName    string `yaml:"opc_application_name"`
	OPCApplicationURI     string `yaml:"opc_application_uri"`

	// S7
	S7Rack int `yaml:"s7_rack"`
	S7Slot int `yaml:"s7_slot"`
}

// TagConfig represents a tag configuration in YAML.
type TagConfig struct {
	ID            string            `yaml:"id"`
	Name          string            `yaml:"name"`
	Description   string            `yaml:"description,omitempty"`
	DataType      string            `yaml:"data_type"`
	ScaleFactor   float64           `yaml:"scale_factor,omitempty"`
	Offset        float64           `yaml:"offset,omitempty"`
	Unit          string            `yaml:"unit,omitempty"`
	TopicSuffix   string            `yaml:"topic_suffix"`
	PollInterval  string            `yaml:"poll_interval,omitempty"`
	DeadbandType  string            `yaml:"deadband_type,omitempty"`
	DeadbandValue float64           `yaml:"deadband_value,omitempty"`
	Enabled       bool              `yaml:"enabled"`
	AccessMode    string            `yaml:"access_mode,omitempty"`
	Metadata      map[string]string `yaml:"metadata,omitempty"`

	// Modbus-specific
	Address       int    `yaml:"address,omitempty"`
	RegisterType  string `yaml:"register_type,omitempty"`
	ByteOrder     string `yaml:"byte_order,omitempty"`
	RegisterCount int    `yaml:"register_count,omitempty"`
	BitPosition   *int   `yaml:"bit_position,omitempty"`

	// OPC UA-specific
	OPCNodeID string `yaml:"opc_node_id,omitempty"`

	// S7-specific
	S7Address string `yaml:"s7_address,omitempty"`
}

// DevicesFile represents the top-level devices configuration file.
type DevicesFile struct {
	Version string         `yaml:"version"`
	Devices []DeviceConfig `yaml:"devices"`
}

// LoadDevices loads device configurations from a YAML file.
func LoadDevices(path string) ([]*domain.Device, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read devices file: %w", err)
	}

	var file DevicesFile
	if err := yaml.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("failed to parse devices file: %w", err)
	}

	// Track seen IDs to detect duplicates
	seenIDs := make(map[string]int)
	devices := make([]*domain.Device, 0, len(file.Devices))

	for idx, dc := range file.Devices {
		// Check for duplicate IDs
		if prevIdx, exists := seenIDs[dc.ID]; exists {
			return nil, fmt.Errorf("duplicate device ID '%s' at index %d (first seen at index %d)", dc.ID, idx, prevIdx)
		}
		seenIDs[dc.ID] = idx

		// Validate protocol-specific connection requirements
		if err := validateConnectionConfig(dc); err != nil {
			return nil, fmt.Errorf("error in device %s: %w", dc.ID, err)
		}

		device, err := convertDeviceConfig(dc)
		if err != nil {
			return nil, fmt.Errorf("error in device %s: %w", dc.ID, err)
		}
		devices = append(devices, device)
	}

	return devices, nil
}

// validateConnectionConfig validates protocol-specific connection requirements.
func validateConnectionConfig(dc DeviceConfig) error {
	protocol := domain.Protocol(dc.Protocol)

	switch protocol {
	case domain.ProtocolModbusTCP, domain.ProtocolModbusRTU:
		if dc.Connection.Host == "" {
			return fmt.Errorf("modbus device requires host")
		}
		if dc.Connection.Port == 0 {
			return fmt.Errorf("modbus device requires port")
		}
		if dc.Connection.SlaveID < 1 || dc.Connection.SlaveID > 247 {
			return fmt.Errorf("modbus slave_id must be between 1 and 247, got %d", dc.Connection.SlaveID)
		}

	case domain.ProtocolOPCUA:
		if dc.Connection.OPCEndpointURL == "" {
			return fmt.Errorf("OPC UA device requires opc_endpoint_url")
		}

	case domain.ProtocolS7:
		if dc.Connection.Host == "" {
			return fmt.Errorf("S7 device requires host")
		}
		if dc.Connection.S7Rack < 0 {
			return fmt.Errorf("S7 rack must be non-negative")
		}
		if dc.Connection.S7Slot < 0 {
			return fmt.Errorf("S7 slot must be non-negative")
		}

	default:
		// Unknown protocol - let domain validation handle it
	}

	return nil
}

// convertDeviceConfig converts a DeviceConfig to a domain.Device.
func convertDeviceConfig(dc DeviceConfig) (*domain.Device, error) {
	// Parse timeout duration
	timeout := 5 * time.Second
	if dc.Connection.Timeout != "" {
		var err error
		timeout, err = time.ParseDuration(dc.Connection.Timeout)
		if err != nil {
			return nil, fmt.Errorf("invalid timeout: %w", err)
		}
	}

	// Parse retry delay
	retryDelay := 100 * time.Millisecond
	if dc.Connection.RetryDelay != "" {
		var err error
		retryDelay, err = time.ParseDuration(dc.Connection.RetryDelay)
		if err != nil {
			return nil, fmt.Errorf("invalid retry delay: %w", err)
		}
	}

	// Convert tags
	tags := make([]domain.Tag, 0, len(dc.Tags))
	for _, tc := range dc.Tags {
		tag, err := convertTagConfig(tc)
		if err != nil {
			return nil, fmt.Errorf("error in tag %s: %w", tc.ID, err)
		}
		tags = append(tags, *tag)
	}

	// Parse poll interval (default 1 second)
	pollInterval := 1 * time.Second
	if dc.PollInterval != "" {
		var err error
		pollInterval, err = time.ParseDuration(dc.PollInterval)
		if err != nil {
			return nil, fmt.Errorf("invalid poll interval: %w", err)
		}
	}

	device := &domain.Device{
		ID:           dc.ID,
		Name:         dc.Name,
		Description:  dc.Description,
		Protocol:     domain.Protocol(dc.Protocol),
		Enabled:      dc.Enabled,
		UNSPrefix:    dc.UNSPrefix,
		PollInterval: pollInterval,
		Tags:         tags,
		Metadata:     dc.Metadata,
		Connection: domain.ConnectionConfig{
			// Common
			Host:       dc.Connection.Host,
			Port:       dc.Connection.Port,
			Timeout:    timeout,
			RetryCount: dc.Connection.RetryCount,
			RetryDelay: retryDelay,

			// Modbus
			SlaveID: uint8(dc.Connection.SlaveID),

			// OPC UA
			OPCEndpointURL:        dc.Connection.OPCEndpointURL,
			OPCSecurityPolicy:     dc.Connection.OPCSecurityPolicy,
			OPCSecurityMode:       dc.Connection.OPCSecurityMode,
			OPCAuthMode:           dc.Connection.OPCAuthMode,
			OPCUsername:           dc.Connection.OPCUsername,
			OPCPassword:           dc.Connection.OPCPassword,
			OPCCertFile:           dc.Connection.OPCCertFile,
			OPCKeyFile:            dc.Connection.OPCKeyFile,
			OPCServerCertFile:     dc.Connection.OPCServerCertFile,
			OPCInsecureSkipVerify: dc.Connection.OPCInsecureSkipVerify,
			OPCAutoSelectEndpoint: dc.Connection.OPCAutoSelectEndpoint,
			OPCApplicationName:    dc.Connection.OPCApplicationName,
			OPCApplicationURI:     dc.Connection.OPCApplicationURI,

			// S7
			S7Rack: dc.Connection.S7Rack,
			S7Slot: dc.Connection.S7Slot,
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// Validate the device
	if err := device.Validate(); err != nil {
		return nil, err
	}

	return device, nil
}

// convertTagConfig converts a TagConfig to a domain.Tag.
func convertTagConfig(tc TagConfig) (*domain.Tag, error) {
	// Parse poll interval if specified
	var pollInterval *time.Duration
	if tc.PollInterval != "" {
		pi, err := time.ParseDuration(tc.PollInterval)
		if err != nil {
			return nil, fmt.Errorf("invalid poll interval: %w", err)
		}
		pollInterval = &pi
	}

	// Convert bit position
	var bitPosition *uint8
	if tc.BitPosition != nil {
		bp := uint8(*tc.BitPosition)
		bitPosition = &bp
	}

	// Set defaults
	scaleFactor := tc.ScaleFactor
	if scaleFactor == 0 {
		scaleFactor = 1.0
	}

	byteOrder := domain.ByteOrder(tc.ByteOrder)
	if byteOrder == "" {
		byteOrder = domain.ByteOrderBigEndian
	}

	registerCount := uint16(tc.RegisterCount)
	if registerCount == 0 {
		registerCount = 1
	}

	// Determine access mode
	accessMode := domain.AccessMode(tc.AccessMode)
	if accessMode == "" {
		accessMode = domain.AccessModeReadOnly
	}

	tag := &domain.Tag{
		ID:            tc.ID,
		Name:          tc.Name,
		Description:   tc.Description,
		DataType:      domain.DataType(tc.DataType),
		ScaleFactor:   scaleFactor,
		Offset:        tc.Offset,
		Unit:          tc.Unit,
		TopicSuffix:   tc.TopicSuffix,
		PollInterval:  pollInterval,
		DeadbandType:  domain.DeadbandType(tc.DeadbandType),
		DeadbandValue: tc.DeadbandValue,
		Enabled:       tc.Enabled,
		AccessMode:    accessMode,
		Metadata:      tc.Metadata,

		// Modbus-specific
		Address:       uint16(tc.Address),
		RegisterType:  domain.RegisterType(tc.RegisterType),
		ByteOrder:     byteOrder,
		RegisterCount: registerCount,
		BitPosition:   bitPosition,

		// OPC UA-specific
		OPCNodeID: tc.OPCNodeID,

		// S7-specific
		S7Address: tc.S7Address,
	}

	return tag, nil
}

// SaveDevices saves device configurations to a YAML file.
func SaveDevices(path string, devices []*domain.Device) error {
	configs := make([]DeviceConfig, 0, len(devices))
	for _, device := range devices {
		configs = append(configs, convertToDeviceConfig(device))
	}

	file := DevicesFile{
		Version: "1.0",
		Devices: configs,
	}

	data, err := yaml.Marshal(&file)
	if err != nil {
		return fmt.Errorf("failed to marshal devices: %w", err)
	}

	// Use 0600 permissions to protect credentials from other local users
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("failed to write devices file: %w", err)
	}

	return nil
}

// convertToDeviceConfig converts a domain.Device to a DeviceConfig.
func convertToDeviceConfig(device *domain.Device) DeviceConfig {
	tags := make([]TagConfig, 0, len(device.Tags))
	for _, tag := range device.Tags {
		tags = append(tags, convertToTagConfig(&tag))
	}

	return DeviceConfig{
		ID:           device.ID,
		Name:         device.Name,
		Description:  device.Description,
		Protocol:     string(device.Protocol),
		Enabled:      device.Enabled,
		UNSPrefix:    device.UNSPrefix,
		PollInterval: device.PollInterval.String(),
		Connection: ConnectionConfig{
			Host:       device.Connection.Host,
			Port:       device.Connection.Port,
			SlaveID:    int(device.Connection.SlaveID),
			Timeout:    device.Connection.Timeout.String(),
			RetryCount: device.Connection.RetryCount,
			RetryDelay: device.Connection.RetryDelay.String(),

			// OPC UA
			OPCEndpointURL:        device.Connection.OPCEndpointURL,
			OPCSecurityPolicy:     device.Connection.OPCSecurityPolicy,
			OPCSecurityMode:       device.Connection.OPCSecurityMode,
			OPCAuthMode:           device.Connection.OPCAuthMode,
			OPCUsername:           device.Connection.OPCUsername,
			OPCPassword:           device.Connection.OPCPassword,
			OPCCertFile:           device.Connection.OPCCertFile,
			OPCKeyFile:            device.Connection.OPCKeyFile,
			OPCServerCertFile:     device.Connection.OPCServerCertFile,
			OPCInsecureSkipVerify: device.Connection.OPCInsecureSkipVerify,
			OPCAutoSelectEndpoint: device.Connection.OPCAutoSelectEndpoint,
			OPCApplicationName:    device.Connection.OPCApplicationName,
			OPCApplicationURI:     device.Connection.OPCApplicationURI,

			// S7
			S7Rack: device.Connection.S7Rack,
			S7Slot: device.Connection.S7Slot,
		},
		Tags:     tags,
		Metadata: device.Metadata,
	}
}

// convertToTagConfig converts a domain.Tag to a TagConfig.
func convertToTagConfig(tag *domain.Tag) TagConfig {
	var pollInterval string
	if tag.PollInterval != nil {
		pollInterval = tag.PollInterval.String()
	}

	var bitPosition *int
	if tag.BitPosition != nil {
		bp := int(*tag.BitPosition)
		bitPosition = &bp
	}

	return TagConfig{
		ID:            tag.ID,
		Name:          tag.Name,
		Description:   tag.Description,
		Address:       int(tag.Address),
		RegisterType:  string(tag.RegisterType),
		DataType:      string(tag.DataType),
		ByteOrder:     string(tag.ByteOrder),
		RegisterCount: int(tag.RegisterCount),
		BitPosition:   bitPosition,
		ScaleFactor:   tag.ScaleFactor,
		Offset:        tag.Offset,
		Unit:          tag.Unit,
		TopicSuffix:   tag.TopicSuffix,
		PollInterval:  pollInterval,
		DeadbandType:  string(tag.DeadbandType),
		DeadbandValue: tag.DeadbandValue,
		Enabled:       tag.Enabled,
		AccessMode:    string(tag.AccessMode),
		Metadata:      tag.Metadata,

		// OPC UA
		OPCNodeID: tag.OPCNodeID,

		// S7
		S7Address: tag.S7Address,
	}
}
