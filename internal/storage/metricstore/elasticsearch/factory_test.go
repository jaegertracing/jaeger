// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package elasticsearch

import (
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
	"github.com/jaegertracing/jaeger/internal/storage/v1"
	"github.com/jaegertracing/jaeger/internal/telemetry"
)

var _ storage.MetricStoreFactory = new(Factory)

func TestNewFactory(t *testing.T) {
	f := NewFactory()
	assert.NotNil(t, f)
	assert.NotNil(t, f.telset)
}

func TestFactory_Initialize(t *testing.T) {
	f := NewFactory()
	settings := telemetry.Settings{
		Logger: zap.NewNop(),
	}
	err := f.Initialize(settings)
	require.NoError(t, err)
	assert.Equal(t, settings, f.telset)
}

func TestCreateMetricsReader(t *testing.T) {
	f := NewFactory()
	require.NoError(t, f.Initialize(telemetry.NoopSettings()))
	assert.NotNil(t, f.telset)

	listener, err := net.Listen("tcp", "localhost:")
	require.NoError(t, err)
	assert.NotNil(t, listener)
	defer listener.Close()

	f.config = config.Configuration{
		Servers: []string{"http://" + listener.Addr().String()},
	}
	reader, err := f.CreateMetricsReader()

	require.NoError(t, err)
	assert.NotNil(t, reader)
}

func TestCreateMetricsReaderError(t *testing.T) {
	f := NewFactory()
	f.config = config.Configuration{
		Servers: []string{"invalid-url"},
	}
	require.NoError(t, f.Initialize(telemetry.NoopSettings()))
	reader, err := f.CreateMetricsReader()
	require.Error(t, err)
	require.Nil(t, reader)
}

func TestNewFactoryWithConfig(t *testing.T) {
	tests := []struct {
		name        string
		cfg         config.Configuration
		expectedErr bool
	}{
		{
			name: "valid config",
			cfg: config.Configuration{
				Servers: []string{"http://localhost:9200"},
			},
			expectedErr: false,
		},
		{
			name: "invalid config - no servers",
			cfg: config.Configuration{
				Servers: []string{},
			},
			expectedErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			telset := telemetry.NoopSettings()
			_, err := NewFactoryWithConfig(tt.cfg, telset)

			if tt.expectedErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
