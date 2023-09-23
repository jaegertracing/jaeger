// Copyright (c) 2023 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerquery

import (
	"context"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/confighttp"
	"go.opentelemetry.io/collector/extension"

	"github.com/jaegertracing/jaeger/ports"
)

const (
	// The value of extension "type" in configuration.
	typeStr = "jaeger_query"
)

func NewFactory() extension.Factory {
	return extension.NewFactory(typeStr, createDefaultConfig, createExtension, component.StabilityLevelBeta)
}

func createDefaultConfig() component.Config {
	return &Config{
		HTTPServerSettings: confighttp.HTTPServerSettings{
			Endpoint: ports.PortToHostPort(ports.QueryHTTP),
		},
	}
}

// createExtension creates the extension based on this config.
func createExtension(_ context.Context, set extension.CreateSettings, cfg component.Config) (extension.Extension, error) {
	return newServer(cfg.(*Config), set.TelemetrySettings), nil
}
