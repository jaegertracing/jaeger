// Copyright (c) 2020 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package http

import (
	"context"

	"github.com/jaegertracing/jaeger/internal/sampling/strategy"
	"github.com/jaegertracing/jaeger/proto-gen/api_v2"
)

// ConfigManager implements ClientConfigManager.
type ConfigManager struct {
	SamplingProvider strategy.Provider
}

// GetSamplingStrategy implements ClientConfigManager.GetSamplingStrategy.
func (c *ConfigManager) GetSamplingStrategy(ctx context.Context, serviceName string) (*api_v2.SamplingStrategyResponse, error) {
	return c.SamplingProvider.GetSamplingStrategy(ctx, serviceName)
}
