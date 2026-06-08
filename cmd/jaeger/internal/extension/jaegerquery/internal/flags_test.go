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
	require.Equal(t, DefaultAIAgentURL, aiCfg.AgentURL)
	require.Equal(t, DefaultAIMaxRequestBodySize, aiCfg.MaxRequestBodySize)
	require.Equal(t, DefaultAIHealthCheckInterval, aiCfg.HealthCheckInterval)
	require.Equal(t, DefaultAIHealthCheckTimeout, aiCfg.HealthCheckTimeout)
	require.NoError(t, aiCfg.Validate())
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

func TestAIConfigValidateRejectsEmptyAgentURL(t *testing.T) {
	cfg := validAIConfig()
	cfg.AgentURL = ""
	require.EqualError(t, cfg.Validate(), "ai.agent_url is required")
}

func TestAIConfigValidateRejectsNonPositiveBodySize(t *testing.T) {
	for _, size := range []int64{0, -1} {
		cfg := validAIConfig()
		cfg.MaxRequestBodySize = size
		require.EqualError(t, cfg.Validate(), "ai.max_request_body_size must be a positive integer")
	}
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
