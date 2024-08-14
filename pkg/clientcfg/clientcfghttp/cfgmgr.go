// Copyright (c) 2020 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package clientcfghttp

import (
	"context"
	"errors"

	"github.com/jaegertracing/jaeger/cmd/collector/app/sampling/samplingstrategy"
	"github.com/jaegertracing/jaeger/proto-gen/api_v2"
	"github.com/jaegertracing/jaeger/thrift-gen/baggage"
)

// ConfigManager implements ClientConfigManager.
type ConfigManager struct {
	SamplingProvider samplingstrategy.Provider
	BaggageManager   baggage.BaggageRestrictionManager
}

// GetSamplingStrategy implements ClientConfigManager.GetSamplingStrategy.
func (c *ConfigManager) GetSamplingStrategy(ctx context.Context, serviceName string) (*api_v2.SamplingStrategyResponse, error) {
	return c.SamplingProvider.GetSamplingStrategy(ctx, serviceName)
}

// GetBaggageRestrictions implements ClientConfigManager.GetBaggageRestrictions.
func (c *ConfigManager) GetBaggageRestrictions(ctx context.Context, serviceName string) ([]*baggage.BaggageRestriction, error) {
	if c.BaggageManager == nil {
		return nil, errors.New("baggage restrictions not implemented")
	}
	return c.BaggageManager.GetBaggageRestrictions(ctx, serviceName)
}
