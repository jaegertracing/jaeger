// Copyright (c) 2018 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package grpc

import (
	"context"

	"github.com/jaegertracing/jaeger/internal/sampling/strategy"
	"github.com/jaegertracing/jaeger/proto-gen/api_v2"
)

// Handler is sampling strategy handler for gRPC.
type Handler struct {
	samplingProvider strategy.Provider
}

// NewHandler creates a handler that controls sampling strategies for services.
func NewHandler(provider strategy.Provider) Handler {
	return Handler{
		samplingProvider: provider,
	}
}

// GetSamplingStrategy returns sampling decision from store.
func (s Handler) GetSamplingStrategy(ctx context.Context, param *api_v2.SamplingStrategyParameters) (*api_v2.SamplingStrategyResponse, error) {
	return s.samplingProvider.GetSamplingStrategy(ctx, param.GetServiceName())
}
