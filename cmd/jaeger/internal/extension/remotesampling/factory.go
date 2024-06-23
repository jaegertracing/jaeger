// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package remotesampling

import (
	"context"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/configgrpc"
	"go.opentelemetry.io/collector/config/confighttp"
	"go.opentelemetry.io/collector/config/confignet"
	"go.opentelemetry.io/collector/extension"

	"github.com/jaegertracing/jaeger/plugin/sampling/strategyprovider/adaptive"
	"github.com/jaegertracing/jaeger/ports"
)

// componentType is the name of this extension in configuration.
var componentType = component.MustNewType("remote_sampling")

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
		HTTP: &confighttp.ServerConfig{
			Endpoint: ports.PortToHostPort(ports.CollectorHTTP + 100),
		},
		GRPC: &configgrpc.ServerConfig{
			NetAddr: confignet.AddrConfig{
				Endpoint:  ports.PortToHostPort(ports.CollectorGRPC + 100),
				Transport: confignet.TransportTypeTCP,
			},
		},
		File: &FileConfig{
			Path: "", // path needs to be specified
		},
		Adaptive: &AdaptiveConfig{
			SamplingStore: "", // storage name needs to be specified
			Options:       adaptive.DefaultOptions(),
		},
	}
}

func createExtension(_ context.Context, set extension.Settings, cfg component.Config) (extension.Extension, error) {
	return newExtension(cfg.(*Config), set.TelemetrySettings), nil
}
