// Copyright (c) 2023 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerquery

import (
	"context"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/extension"

	"github.com/jaegertracing/jaeger/cmd/query/app"
)

// componentType is the name of this extension in configuration.
var componentType = component.MustNewType("jaeger_query")

// ID is the identifier of this extension.
var ID = component.NewID(componentType)

func NewFactory() extension.Factory {
	return extension.NewFactory(componentType, createDefaultConfig, createExtension, component.StabilityLevelBeta)
}

func createDefaultConfig() component.Config {
	queryOptions := app.DefaultQueryOptions()
	// Tracing is off by default in v1, but we want it on for v2
	queryOptions.EnableTracing = true
	return &Config{
		QueryOptions: queryOptions,
	}
}

// createExtension creates the extension based on this config.
func createExtension(_ context.Context, set extension.Settings, cfg component.Config) (extension.Extension, error) {
	return newServer(cfg.(*Config), set.TelemetrySettings), nil
}
