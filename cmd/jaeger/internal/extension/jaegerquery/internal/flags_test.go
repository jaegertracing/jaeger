// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
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
	require.Equal(t, DefaultHealthCheckInterval, aiCfg.HealthCheckInterval)
	require.Equal(t, DefaultHealthCheckTimeout, aiCfg.HealthCheckTimeout)
	require.NoError(t, aiCfg.Validate())
}

// validAIConfig returns an AIConfig that passes Validate; tests then mutate
// the field they care about to exercise one validation rule at a time.
func validAIConfig() AIConfig {
	return AIConfig{AgentURL: "ws://localhost:16688"}
}

func TestAIConfigValidateRejectsEmptyAgentURL(t *testing.T) {
	cfg := AIConfig{}
	require.EqualError(t, cfg.Validate(), "ai.agent_url is required")
}

func TestAIConfigValidateRejectsNegativeBodySize(t *testing.T) {
	cfg := validAIConfig()
	cfg.MaxRequestBodySize = -1
	require.Error(t, cfg.Validate())
}

func TestAIConfigValidateDefaultsZeroBodySize(t *testing.T) {
	cfg := validAIConfig()
	require.NoError(t, cfg.Validate())
	require.Equal(t, DefaultMaxRequestBodySize, cfg.MaxRequestBodySize)
}

func TestAIConfigValidateAcceptsPositiveBodySize(t *testing.T) {
	cfg := validAIConfig()
	cfg.MaxRequestBodySize = 1
	require.NoError(t, cfg.Validate())
	require.Equal(t, int64(1), cfg.MaxRequestBodySize)
}

func TestAIConfigValidateDefaultsHealthCheckFields(t *testing.T) {
	cfg := validAIConfig()
	require.NoError(t, cfg.Validate())
	require.Equal(t, DefaultHealthCheckInterval, cfg.HealthCheckInterval)
	require.Equal(t, DefaultHealthCheckTimeout, cfg.HealthCheckTimeout)
}

func TestAIConfigValidateRejectsNegativeHealthCheckTimeout(t *testing.T) {
	cfg := validAIConfig()
	cfg.HealthCheckTimeout = -time.Second
	require.Error(t, cfg.Validate())
}

func TestAIConfigValidatePreservesNegativeHealthCheckInterval(t *testing.T) {
	// A negative interval is a deliberate "disable" signal — Validate must
	// leave it as-is rather than overwriting with the default.
	cfg := validAIConfig()
	cfg.HealthCheckInterval = -time.Second
	require.NoError(t, cfg.Validate())
	require.Equal(t, -time.Second, cfg.HealthCheckInterval)
}
