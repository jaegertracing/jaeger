// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package remotesampling

import (
	"context"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/configgrpc"
	"go.opentelemetry.io/collector/config/confighttp"
	"go.opentelemetry.io/collector/config/confignet"
	"go.opentelemetry.io/collector/config/configoptional"
	"go.opentelemetry.io/collector/extension"

	"github.com/jaegertracing/jaeger/internal/sampling/samplingstrategy/adaptive"
	"github.com/jaegertracing/jaeger/internal/sampling/samplingstrategy/file"
	"github.com/jaegertracing/jaeger/ports"
)

// ComponentType is the name of this extension in configuration.
var ComponentType = component.MustNewType("remote_sampling")

var ID = component.NewID(ComponentType)

// NewFactory creates a factory for the jaeger remote sampling extension.
func NewFactory() extension.Factory {
	return extension.NewFactory(
		ComponentType,
		createDefaultConfig,
		createExtension,
		component.StabilityLevelBeta,
	)
}

func createDefaultConfig() component.Config {
	return &Config{
		HTTP: configoptional.Default(confighttp.ServerConfig{
			NetAddr: confignet.AddrConfig{
				Endpoint:  ports.PortToHostPort(ports.CollectorV2SamplingHTTP),
				Transport: confignet.TransportTypeTCP,
			},
		}),
		GRPC: configoptional.Default(configgrpc.ServerConfig{
			NetAddr: confignet.AddrConfig{
				Endpoint:  ports.PortToHostPort(ports.CollectorV2SamplingGRPC),
				Transport: confignet.TransportTypeTCP,
			},
		}),
		// use Default() to provide defaults when users specifyi file: or adaptive: in YAML, this will not violate mutual exclusivity
		File: configoptional.Default(FileConfig{
			DefaultSamplingProbability: file.DefaultSamplingProbability,
		}),
		Adaptive: configoptional.Default(AdaptiveConfig{
			Options: adaptive.DefaultOptions(),
		}),
	}
}

func createExtension(_ context.Context, set extension.Settings, cfg component.Config) (extension.Extension, error) {
	return newExtension(cfg.(*Config), set.TelemetrySettings), nil
}
