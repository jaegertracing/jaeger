// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegermcp

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/extension"
)

func TestNewFactory(t *testing.T) {
	factory := NewFactory()
	assert.Equal(t, componentType, factory.Type())
	assert.Equal(t, factory.CreateDefaultConfig(), createDefaultConfig())
}

func TestCreateExtension(t *testing.T) {
	set := extension.Settings{
		ID: ID,
	}
	cfg := createDefaultConfig()
	ext, err := createExtension(context.Background(), set, cfg)

	require.NoError(t, err)
	assert.NotNil(t, ext)
}

func TestCreateDefaultConfig(t *testing.T) {
	cfg := createDefaultConfig()
	assert.NotNil(t, cfg)

	mcpCfg, ok := cfg.(*Config)
	require.True(t, ok)

	assert.Equal(t, ":16687", mcpCfg.HTTP.NetAddr.Endpoint)
	assert.Equal(t, "jaeger", mcpCfg.ServerName)
	// server_version will be empty in tests since it's set at build time
	assert.Equal(t, 20, mcpCfg.MaxSpanDetailsPerRequest)
	assert.Equal(t, 100, mcpCfg.MaxSearchResults)
}
