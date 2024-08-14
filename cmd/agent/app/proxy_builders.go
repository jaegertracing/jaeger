// Copyright (c) 2020 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"

	"github.com/jaegertracing/jaeger/cmd/agent/app/reporter/grpc"
)

// GRPCCollectorProxyBuilder creates CollectorProxyBuilder for GRPC reporter
func GRPCCollectorProxyBuilder(builder *grpc.ConnBuilder) CollectorProxyBuilder {
	return func(ctx context.Context, opts ProxyBuilderOptions) (proxy CollectorProxy, err error) {
		return grpc.NewCollectorProxy(ctx, builder, opts.AgentTags, opts.Metrics, opts.Logger)
	}
}
