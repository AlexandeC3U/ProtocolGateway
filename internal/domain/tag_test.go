package domain_test

import (
	"testing"
	"time"

	"github.com/nexus-edge/protocol-gateway/internal/domain"
)

func TestTag_Validate(t *testing.T) {
	tests := []struct {
		name    string
		tag     domain.Tag
		wantErr bool
	}{
		{
			name: "valid Modbus tag",
			tag: domain.Tag{
				ID:            "temp-001",
				Name:          "Temperature Sensor 1",
				DataType:      domain.DataTypeFloat32,
				RegisterType:  domain.RegisterTypeHoldingRegister,
				Address:       100,
				RegisterCount: 2,
				TopicSuffix:   "temperature/sensor1",
			},
			wantErr: false,
		},
		{
			name: "valid OPC UA tag",
			tag: domain.Tag{
				ID:          "opc-temp",
				Name:        "OPC Temperature",
				DataType:    domain.DataTypeFloat64,
				OPCNodeID:   "ns=2;s=Temperature",
				TopicSuffix: "opcua/temperature",
			},
			wantErr: false,
		},
		{
			name: "valid S7 tag",
			tag: domain.Tag{
				ID:          "s7-temp",
				Name:        "S7 Temperature",
				DataType:    domain.DataTypeFloat32,
				S7Address:   "DB1.DBD0",
				TopicSuffix: "s7/temperature",
			},
			wantErr: false,
		},
		{
			name: "missing tag ID",
			tag: domain.Tag{
				Name:        "Test Tag",
				DataType:    domain.DataTypeBool,
				TopicSuffix: "test",
			},
			wantErr: true,
		},
		{
			name: "missing tag name",
			tag: domain.Tag{
				ID:          "tag-001",
				DataType:    domain.DataTypeBool,
				TopicSuffix: "test",
			},
			wantErr: true,
		},
		{
			name: "missing topic suffix",
			tag: domain.Tag{
				ID:       "tag-001",
				Name:     "Test Tag",
				DataType: domain.DataTypeBool,
			},
			wantErr: true,
		},
		{
			name: "missing data type",
			tag: domain.Tag{
				ID:          "tag-001",
				Name:        "Test Tag",
				TopicSuffix: "test",
			},
			wantErr: true,
		},
		{
			name: "Modbus tag missing register type",
			tag: domain.Tag{
				ID:          "modbus-001",
				Name:        "Modbus Tag",
				DataType:    domain.DataTypeUInt16,
				TopicSuffix: "test",
				// No OPCNodeID or S7Address, so it's Modbus
				// Missing RegisterType
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.tag.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Tag.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestTag_ExpectedRegisterCount(t *testing.T) {
	tests := []struct {
		dataType domain.DataType
		want     uint16
	}{
		{domain.DataTypeBool, 1},
		{domain.DataTypeInt16, 1},
		{domain.DataTypeUInt16, 1},
		{domain.DataTypeInt32, 2},
		{domain.DataTypeUInt32, 2},
		{domain.DataTypeFloat32, 2},
		{domain.DataTypeInt64, 4},
		{domain.DataTypeUInt64, 4},
		{domain.DataTypeFloat64, 4},
	}

	for _, tt := range tests {
		t.Run(string(tt.dataType), func(t *testing.T) {
			tag := &domain.Tag{DataType: tt.dataType}
			if got := tag.ExpectedRegisterCount(); got != tt.want {
				t.Errorf("Tag.ExpectedRegisterCount() for %s = %v, want %v", tt.dataType, got, tt.want)
			}
		})
	}
}

func TestTag_IsWritable(t *testing.T) {
	tests := []struct {
		name string
		tag  domain.Tag
		want bool
	}{
		{
			name: "coil is writable",
			tag:  domain.Tag{RegisterType: domain.RegisterTypeCoil},
			want: true,
		},
		{
			name: "holding register is writable",
			tag:  domain.Tag{RegisterType: domain.RegisterTypeHoldingRegister},
			want: true,
		},
		{
			name: "discrete input is not writable",
			tag:  domain.Tag{RegisterType: domain.RegisterTypeDiscreteInput},
			want: false,
		},
		{
			name: "input register is not writable",
			tag:  domain.Tag{RegisterType: domain.RegisterTypeInputRegister},
			want: false,
		},
		{
			name: "explicit read-only access mode",
			tag:  domain.Tag{AccessMode: domain.AccessModeReadOnly, RegisterType: domain.RegisterTypeHoldingRegister},
			want: false,
		},
		{
			name: "explicit write access mode",
			tag:  domain.Tag{AccessMode: domain.AccessModeWriteOnly},
			want: true,
		},
		{
			name: "explicit read-write access mode",
			tag:  domain.Tag{AccessMode: domain.AccessModeReadWrite},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.tag.IsWritable(); got != tt.want {
				t.Errorf("Tag.IsWritable() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTag_GetEffectivePollInterval(t *testing.T) {
	deviceDefault := 5 * time.Second
	tagOverride := 1 * time.Second

	tests := []struct {
		name          string
		tagInterval   *time.Duration
		deviceDefault time.Duration
		want          time.Duration
	}{
		{
			name:          "use device default",
			tagInterval:   nil,
			deviceDefault: deviceDefault,
			want:          deviceDefault,
		},
		{
			name:          "use tag override",
			tagInterval:   &tagOverride,
			deviceDefault: deviceDefault,
			want:          tagOverride,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tag := &domain.Tag{PollInterval: tt.tagInterval}
			if got := tag.GetEffectivePollInterval(tt.deviceDefault); got != tt.want {
				t.Errorf("Tag.GetEffectivePollInterval() = %v, want %v", got, tt.want)
			}
		})
	}
}
