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

// componentType is the name of this extension in configuration.
const componentType = component.Type("jaeger_query")

// ID is the identifier of this extension.
var ID = component.NewID(componentType)

func NewFactory() extension.Factory {
	return extension.NewFactory(componentType, createDefaultConfig, createExtension, component.StabilityLevelBeta)
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
