// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegermcp

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component/componenttest"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery"
)

func TestServerLifecycle(t *testing.T) {
	// Since we're not actually accessing storage in Phase 1,
	// we just need a basic host for the lifecycle test
	host := componenttest.NewNopHost()

	tests := []struct {
		name          string
		config        *Config
		expectedError string
	}{
		{
			name: "successful start and shutdown",
			config: &Config{
				HTTP: createDefaultConfig().(*Config).HTTP,
			},
			expectedError: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			telset := componenttest.NewNopTelemetrySettings()
			server := newServer(tt.config, telset)
			require.NotNil(t, server)

			// Test Start
			err := server.Start(context.Background(), host)
			if tt.expectedError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
				return
			}
			require.NoError(t, err)

			// Test Shutdown
			err = server.Shutdown(context.Background())
			assert.NoError(t, err)
		})
	}
}

func TestServerDependencies(t *testing.T) {
	server := &server{}
	deps := server.Dependencies()
	require.Len(t, deps, 1)
	assert.Equal(t, jaegerquery.ID, deps[0])
}

func TestShutdownWithoutStart(t *testing.T) {
	telset := componenttest.NewNopTelemetrySettings()
	server := newServer(createDefaultConfig().(*Config), telset)

	err := server.Shutdown(context.Background())
	assert.NoError(t, err)
}
