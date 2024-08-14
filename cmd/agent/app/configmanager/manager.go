// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package configmanager

import (
	"context"

	"github.com/jaegertracing/jaeger/proto-gen/api_v2"
	"github.com/jaegertracing/jaeger/thrift-gen/baggage"
)

// TODO this interface could be moved to pkg/clientcfg, along with grpc proxy,
// but not the metrics wrapper (because its metric names are specific to agent).

// ClientConfigManager decides:
// 1) which sampling strategy a given service should be using
// 2) which baggage restrictions a given service should be using.
type ClientConfigManager interface {
	GetSamplingStrategy(ctx context.Context, serviceName string) (*api_v2.SamplingStrategyResponse, error)
	GetBaggageRestrictions(ctx context.Context, serviceName string) ([]*baggage.BaggageRestriction, error)
}
