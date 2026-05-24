// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/internal/jaegerai"
)

func TestDefaultQueryOptions(t *testing.T) {
	qo := DefaultQueryOptions()
	require.Equal(t, ":16686", qo.HTTP.NetAddr.Endpoint)
	require.Equal(t, ":16685", qo.GRPC.NetAddr.Endpoint)
	require.EqualValues(t, "tcp", qo.GRPC.NetAddr.Transport)
	require.False(t, qo.AI.HasValue())
	aiCfg := qo.AI.GetOrInsertDefault()
	require.NotNil(t, aiCfg)
	require.Equal(t, "ws://localhost:16688", aiCfg.AgentURL)
	require.Equal(t, int64(1<<20), aiCfg.MaxRequestBodySize)
	require.NoError(t, aiCfg.Validate())
}

func TestAIConfigValidateRejectsNegativeBodySize(t *testing.T) {
	cfg := AIConfig{MaxRequestBodySize: -1}
	require.Error(t, cfg.Validate())
}

func TestAIConfigValidateDefaultsZeroBodySize(t *testing.T) {
	cfg := AIConfig{MaxRequestBodySize: 0}
	require.NoError(t, cfg.Validate())
	require.Equal(t, DefaultMaxRequestBodySize, cfg.MaxRequestBodySize)
}

func TestAIConfigValidateAcceptsPositiveBodySize(t *testing.T) {
	cfg := AIConfig{MaxRequestBodySize: 1}
	require.NoError(t, cfg.Validate())
	require.Equal(t, int64(1), cfg.MaxRequestBodySize)
}

func TestAIConfigValidateMcpServers(t *testing.T) {
	// Every realistic operator mistake the tightened URL check catches —
	// scheme typo, missing host, plain garbage — should surface as a
	// config error at startup, not as a confusing mid-conversation
	// failure from the agent. Empty Name / URL go through the same
	// surface so the user sees one well-formed error message.
	for _, tc := range []struct {
		name    string
		servers []jaegerai.McpServerConfig
		wantErr string
	}{
		{
			name:    "missing name",
			servers: []jaegerai.McpServerConfig{{URL: "http://localhost:16687/mcp"}},
			wantErr: "ai.mcp_servers[0].name is required",
		},
		{
			name:    "missing url",
			servers: []jaegerai.McpServerConfig{{Name: "jaeger"}},
			wantErr: "ai.mcp_servers[0].url is required",
		},
		{
			name:    "url without scheme is rejected",
			servers: []jaegerai.McpServerConfig{{Name: "jaeger", URL: "localhost:16687/mcp"}},
			wantErr: "must be an http(s) URL with host",
		},
		{
			name:    "non-http scheme is rejected",
			servers: []jaegerai.McpServerConfig{{Name: "jaeger", URL: "ftp://example/mcp"}},
			wantErr: "must be an http(s) URL with host",
		},
		{
			name:    "http url with no host is rejected",
			servers: []jaegerai.McpServerConfig{{Name: "jaeger", URL: "http:///mcp"}},
			wantErr: "must be an http(s) URL with host",
		},
		{
			name:    "garbage url is rejected",
			servers: []jaegerai.McpServerConfig{{Name: "jaeger", URL: "not a url"}},
			wantErr: "must be an http(s) URL with host",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := AIConfig{McpServers: tc.servers}
			err := cfg.Validate()
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

func TestAIConfigValidateAcceptsWellFormedMcpServers(t *testing.T) {
	cfg := AIConfig{
		McpServers: []jaegerai.McpServerConfig{
			{Name: "jaeger", URL: "http://localhost:16687/mcp"},
			{Name: "remote", URL: "https://mcp.example.com:8443/mcp"},
		},
	}
	require.NoError(t, cfg.Validate())
}
