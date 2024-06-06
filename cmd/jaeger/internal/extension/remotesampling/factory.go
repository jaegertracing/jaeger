// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package remotesampling

import (
	"context"
	"time"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/configgrpc"
	"go.opentelemetry.io/collector/config/confighttp"
	"go.opentelemetry.io/collector/config/confignet"
	"go.opentelemetry.io/collector/extension"

	"github.com/jaegertracing/jaeger/ports"
)

// componentType is the name of this extension in configuration.
var componentType = component.MustNewType("remote_sampling")

const (
	defaultAggregationBuckets           = 10
	defaultInitialSamplingProbability   = 0.001
	defaultMinSamplesPerSecond          = 1.0 / float64(time.Minute/time.Second) // once every 1 minute
	defaultLeaderLeaseRefreshInterval   = 5 * time.Second
	defaultFollowerLeaseRefreshInterval = 60 * time.Second
)

// NewFactory creates a factory for the jaeger remote sampling extension.
func NewFactory() extension.Factory {
	return extension.NewFactory(
		componentType,
		createDefaultConfig,
		createExtension,
		component.StabilityLevelBeta,
	)
}

func createDefaultConfig() component.Config {
	return &Config{
		HTTP: HTTPConfig{
			ServerConfig: confighttp.ServerConfig{
				Endpoint: ports.PortToHostPort(ports.CollectorHTTP),
			},
		},
		GRPC: GRPCConfig{
			ServerConfig: configgrpc.ServerConfig{
				NetAddr: confignet.AddrConfig{
					Endpoint:  ports.PortToHostPort(ports.CollectorGRPC),
					Transport: confignet.TransportTypeTCP,
				},
			},
		},
		File: FileConfig{
			Path: "",
		},
		Adaptive: AdaptiveConfig{
			InitialSamplingProbability:   defaultInitialSamplingProbability,
			MinSamplesPerSecond:          defaultMinSamplesPerSecond,
			LeaderLeaseRefreshInterval:   defaultLeaderLeaseRefreshInterval,
			FollowerLeaseRefreshInterval: defaultFollowerLeaseRefreshInterval,
			AggregationBuckets:           defaultAggregationBuckets,
		},
	}
}

func createExtension(_ context.Context, set extension.CreateSettings, cfg component.Config) (extension.Extension, error) {
	return newExtension(cfg.(*Config), set.TelemetrySettings), nil
}
