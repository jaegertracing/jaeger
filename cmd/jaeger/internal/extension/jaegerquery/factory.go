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
	return &Config{
		QueryOptions: app.DefaultQueryOptions(),
	}
}

// createExtension creates the extension based on this config.
func createExtension(_ context.Context, set extension.Settings, cfg component.Config) (extension.Extension, error) {
	return newServer(cfg.(*Config), set.TelemetrySettings), nil
}
