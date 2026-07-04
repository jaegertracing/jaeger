// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/cmd/jaeger/internal/extension/jaegerquery/internal/mcptools"
)

func TestDefaultQueryOptions(t *testing.T) {
	qo := DefaultQueryOptions()
	require.Equal(t, ":16686", qo.HTTP.NetAddr.Endpoint)
	require.Equal(t, ":16685", qo.GRPC.NetAddr.Endpoint)
	require.EqualValues(t, "tcp", qo.GRPC.NetAddr.Transport)
	require.False(t, qo.AI.HasValue())
	aiCfg := qo.AI.GetOrInsertDefault()
	require.NotNil(t, aiCfg)
	require.Equal(t, DefaultAIAgentURL, aiCfg.AgentURL)
	require.Equal(t, DefaultAIMaxRequestBodySize, aiCfg.MaxRequestBodySize)
	require.Equal(t, DefaultAIHealthCheckInterval, aiCfg.HealthCheckInterval)
	require.Equal(t, DefaultAIHealthCheckTimeout, aiCfg.HealthCheckTimeout)
	require.NoError(t, aiCfg.Validate())

	require.False(t, qo.OTLPProxy.HasValue())
	otlpCfg := qo.OTLPProxy.GetOrInsertDefault()
	require.NotNil(t, otlpCfg)
	require.Equal(t, DefaultOTLPProxyTarget, otlpCfg.Target)
	require.NoError(t, otlpCfg.Validate())
}

func TestOTLPProxyConfigValidate(t *testing.T) {
	require.NoError(t, (&OTLPProxyConfig{Target: DefaultOTLPProxyTarget}).Validate())
	require.EqualError(t, (&OTLPProxyConfig{Target: ""}).Validate(), "otlp_proxy.target is required")
}

// validAIConfig returns an AIConfig that passes Validate; tests mutate the
// field they care about to exercise one rule at a time. The factory-default
// values mirror what configoptional.Default(...) seeds at runtime.
func validAIConfig() AIConfig {
	return AIConfig{
		AgentURL:            DefaultAIAgentURL,
		MaxRequestBodySize:  DefaultAIMaxRequestBodySize,
		HealthCheckInterval: DefaultAIHealthCheckInterval,
		HealthCheckTimeout:  DefaultAIHealthCheckTimeout,
	}
}

func TestAIConfigValidateAcceptsDefaults(t *testing.T) {
	cfg := validAIConfig()
	require.NoError(t, cfg.Validate())
}

func TestAIConfigValidateRejectsEmptyAgentURLWithoutMCP(t *testing.T) {
	cfg := validAIConfig()
	cfg.AgentURL = ""
	require.EqualError(t, cfg.Validate(), "ai requires agent_url (AI chat) or enable_mcp (telemetry MCP tools)")
}

func TestAIConfigValidateAcceptsMCPOnly(t *testing.T) {
	cfg := validAIConfig()
	cfg.AgentURL = ""
	cfg.EnableMCP = true
	require.NoError(t, cfg.Validate())
}

func TestAIConfigValidateRejectsNonPositiveBodySize(t *testing.T) {
	for _, size := range []int64{0, -1} {
		cfg := validAIConfig()
		cfg.MaxRequestBodySize = size
		require.EqualError(t, cfg.Validate(), "ai.max_request_body_size must be a positive integer")
	}
}

func TestAIConfigValidateAcceptsMCPLimits(t *testing.T) {
	cfg := validAIConfig()
	cfg.EnableMCP = true
	// Zero means "use the default".
	require.NoError(t, cfg.Validate())
	// Explicit in-range values are accepted.
	cfg.MCPMaxReadFileSize = 1 << 20
	cfg.MCPMaxSearchResults = 250
	cfg.MCPMaxSpanDetailsPerRequest = 50
	require.NoError(t, cfg.Validate())
}

func TestAIConfigValidateRejectsInvalidMCPLimits(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*AIConfig)
		wantErr string
	}{
		{"read file size negative", func(c *AIConfig) { c.MCPMaxReadFileSize = -1 }, "ai.mcp_max_read_file_size must be between 0 and 10485760"},
		{"read file size over limit", func(c *AIConfig) { c.MCPMaxReadFileSize = 10485761 }, "ai.mcp_max_read_file_size must be between 0 and 10485760"},
		{"search results negative", func(c *AIConfig) { c.MCPMaxSearchResults = -1 }, "ai.mcp_max_search_results must be between 0 and 1000"},
		{"search results over limit", func(c *AIConfig) { c.MCPMaxSearchResults = 1001 }, "ai.mcp_max_search_results must be between 0 and 1000"},
		{"span details negative", func(c *AIConfig) { c.MCPMaxSpanDetailsPerRequest = -1 }, "ai.mcp_max_span_details_per_request must be between 0 and 100"},
		{"span details over limit", func(c *AIConfig) { c.MCPMaxSpanDetailsPerRequest = 101 }, "ai.mcp_max_span_details_per_request must be between 0 and 100"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validAIConfig()
			tt.mutate(&cfg)
			require.EqualError(t, cfg.Validate(), tt.wantErr)
		})
	}
}

func TestAIConfigMCPConfigAppliesOverrides(t *testing.T) {
	defaults := mcptools.DefaultConfig()

	// Zero limits keep the mcptools defaults.
	zeroCfg := validAIConfig()
	got := zeroCfg.MCPConfig()
	require.Equal(t, defaults.MaxReadFileSize, got.MaxReadFileSize)
	require.Equal(t, defaults.MaxSearchResults, got.MaxSearchResults)
	require.Equal(t, defaults.MaxSpanDetailsPerRequest, got.MaxSpanDetailsPerRequest)

	// Non-zero limits override only their own field.
	cfg := validAIConfig()
	cfg.MCPMaxReadFileSize = 1 << 20
	cfg.MCPMaxSearchResults = 250
	cfg.MCPMaxSpanDetailsPerRequest = 50
	got = cfg.MCPConfig()
	require.Equal(t, int64(1<<20), got.MaxReadFileSize)
	require.Equal(t, 250, got.MaxSearchResults)
	require.Equal(t, 50, got.MaxSpanDetailsPerRequest)
	require.Equal(t, defaults.ServerName, got.ServerName)
}

func TestAIConfigValidateAcceptsZeroHealthCheckIntervalAsDisable(t *testing.T) {
	cfg := validAIConfig()
	cfg.HealthCheckInterval = 0
	// Timeout becomes irrelevant when the checker is disabled; the validator
	// must not require a positive timeout in that case.
	cfg.HealthCheckTimeout = 0
	require.NoError(t, cfg.Validate())
}

func TestAIConfigValidateRejectsNegativeHealthCheckInterval(t *testing.T) {
	cfg := validAIConfig()
	cfg.HealthCheckInterval = -time.Second
	require.EqualError(t, cfg.Validate(),
		"ai.health_check_interval must not be negative (0 disables the health checker)")
}

func TestAIConfigValidateRejectsNonPositiveHealthCheckTimeoutWhenEnabled(t *testing.T) {
	for _, timeout := range []time.Duration{0, -time.Second} {
		cfg := validAIConfig()
		cfg.HealthCheckTimeout = timeout
		require.EqualError(t, cfg.Validate(),
			"ai.health_check_timeout must be positive when health_check_interval is positive")
	}
}
