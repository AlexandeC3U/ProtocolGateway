package domain_test

import (
	"testing"
	"time"

	"github.com/nexus-edge/protocol-gateway/internal/domain"
)

func TestDevice_Validate(t *testing.T) {
	tests := []struct {
		name    string
		device  domain.Device
		wantErr error
	}{
		{
			name: "valid device",
			device: domain.Device{
				ID:           "plc-001",
				Name:         "Production PLC",
				Protocol:     domain.ProtocolModbusTCP,
				PollInterval: 1 * time.Second,
				UNSPrefix:    "plant1/line1/plc1",
				Tags: []domain.Tag{
					{
						ID:           "temp",
						Name:         "Temperature",
						DataType:     domain.DataTypeFloat32,
						RegisterType: domain.RegisterTypeHoldingRegister,
						TopicSuffix:  "temperature",
						Enabled:      true,
					},
				},
			},
			wantErr: nil,
		},
		{
			name: "missing device ID",
			device: domain.Device{
				Name:         "Test Device",
				Protocol:     domain.ProtocolModbusTCP,
				PollInterval: 1 * time.Second,
				UNSPrefix:    "test/device",
				Tags:         []domain.Tag{{ID: "tag1", Name: "Tag 1", DataType: domain.DataTypeBool, TopicSuffix: "tag1"}},
			},
			wantErr: domain.ErrDeviceIDRequired,
		},
		{
			name: "missing device name",
			device: domain.Device{
				ID:           "test-001",
				Protocol:     domain.ProtocolModbusTCP,
				PollInterval: 1 * time.Second,
				UNSPrefix:    "test/device",
				Tags:         []domain.Tag{{ID: "tag1", Name: "Tag 1", DataType: domain.DataTypeBool, TopicSuffix: "tag1"}},
			},
			wantErr: domain.ErrDeviceNameRequired,
		},
		{
			name: "missing protocol",
			device: domain.Device{
				ID:           "test-001",
				Name:         "Test Device",
				PollInterval: 1 * time.Second,
				UNSPrefix:    "test/device",
				Tags:         []domain.Tag{{ID: "tag1", Name: "Tag 1", DataType: domain.DataTypeBool, TopicSuffix: "tag1"}},
			},
			wantErr: domain.ErrProtocolRequired,
		},
		{
			name: "no tags defined",
			device: domain.Device{
				ID:           "test-001",
				Name:         "Test Device",
				Protocol:     domain.ProtocolModbusTCP,
				PollInterval: 1 * time.Second,
				UNSPrefix:    "test/device",
				Tags:         []domain.Tag{},
			},
			wantErr: domain.ErrNoTagsDefined,
		},
		{
			name: "poll interval too short",
			device: domain.Device{
				ID:           "test-001",
				Name:         "Test Device",
				Protocol:     domain.ProtocolModbusTCP,
				PollInterval: 50 * time.Millisecond, // Too short
				UNSPrefix:    "test/device",
				Tags:         []domain.Tag{{ID: "tag1", Name: "Tag 1", DataType: domain.DataTypeBool, TopicSuffix: "tag1"}},
			},
			wantErr: domain.ErrPollIntervalTooShort,
		},
		{
			name: "missing UNS prefix",
			device: domain.Device{
				ID:           "test-001",
				Name:         "Test Device",
				Protocol:     domain.ProtocolModbusTCP,
				PollInterval: 1 * time.Second,
				Tags:         []domain.Tag{{ID: "tag1", Name: "Tag 1", DataType: domain.DataTypeBool, TopicSuffix: "tag1"}},
			},
			wantErr: domain.ErrUNSPrefixRequired,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.device.Validate()
			if err != tt.wantErr {
				t.Errorf("Device.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestDevice_GetAddress(t *testing.T) {
	tests := []struct {
		name       string
		connection domain.ConnectionConfig
		want       string
	}{
		{
			name: "TCP address",
			connection: domain.ConnectionConfig{
				Host: "192.168.1.100",
				Port: 502,
			},
			want: "192.168.1.100",
		},
		{
			name: "serial port",
			connection: domain.ConnectionConfig{
				SerialPort: "/dev/ttyUSB0",
			},
			want: "/dev/ttyUSB0",
		},
		{
			name: "prefer host over serial",
			connection: domain.ConnectionConfig{
				Host:       "10.0.0.1",
				SerialPort: "/dev/ttyUSB0",
			},
			want: "10.0.0.1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			device := &domain.Device{
				Connection: tt.connection,
			}
			if got := device.GetAddress(); got != tt.want {
				t.Errorf("Device.GetAddress() = %v, want %v", got, tt.want)
			}
		})
	}
}
