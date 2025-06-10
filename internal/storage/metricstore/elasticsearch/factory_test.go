// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package elasticsearch

import (
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
	"github.com/jaegertracing/jaeger/internal/storage/v1"
	"github.com/jaegertracing/jaeger/internal/telemetry"
)

var _ storage.BaseMetricStoreFactory = new(Factory)

func TestCreateMetricsReader(t *testing.T) {
	listener, err := net.Listen("tcp", "localhost:")
	require.NoError(t, err)
	assert.NotNil(t, listener)
	defer listener.Close()

	cfg := config.Configuration{
		Servers: []string{"http://" + listener.Addr().String()},
	}
	f, err := NewFactory(cfg, telemetry.NoopSettings())
	require.NoError(t, err)
	require.NotNil(t, f)

	reader, err := f.CreateMetricsReader()

	require.NoError(t, err)
	assert.NotNil(t, reader)
}

func TestNewFactory(t *testing.T) {
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
			_, err := NewFactory(tt.cfg, telset)

			if tt.expectedErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
