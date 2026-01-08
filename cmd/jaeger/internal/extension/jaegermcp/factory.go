// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegermcp

import (
	"context"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/confighttp"
	"go.opentelemetry.io/collector/extension"
)

// componentType is the name of this extension in configuration.
var componentType = component.MustNewType("jaeger_mcp")

// ID is the identifier of this extension.
var ID = component.NewID(componentType)

// NewFactory creates a factory for the Jaeger MCP extension.
func NewFactory() extension.Factory {
	return extension.NewFactory(
		componentType,
		createDefaultConfig,
		createExtension,
		component.StabilityLevelAlpha,
	)
}

// createDefaultConfig creates the default configuration for the extension.
func createDefaultConfig() component.Config {
	return &Config{
		HTTP: confighttp.ServerConfig{
			Endpoint: "0.0.0.0:4320",
		},
		ServerName:               "jaeger",
		ServerVersion:            "dev",
		MaxSpanDetailsPerRequest: 20,
		MaxSearchResults:         100,
	}
}

// createExtension creates the extension based on this config.
func createExtension(_ context.Context, set extension.Settings, cfg component.Config) (extension.Extension, error) {
	return newServer(cfg.(*Config), set.TelemetrySettings), nil
}
