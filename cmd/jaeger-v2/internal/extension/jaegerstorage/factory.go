// Copyright (c) 2023 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerstorage

import (
	"context"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/extension"

	memoryCfg "github.com/jaegertracing/jaeger/pkg/memory/config"
)

const (
	// componentType is the name of this extension in configuration.
	componentType = component.Type("jaeger_storage")

	// DefaultMemoryStore is the name of the memory storage included in the default configuration.
	DefaultMemoryStore = "memstore"
)

// ID is the identifier of this extension.
var ID = component.NewID(componentType)

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
		Memory: map[string]memoryCfg.Configuration{
			DefaultMemoryStore: {
				MaxTraces: 100_000,
			},
		},
	}
}

// createExtension creates the extension based on this config.
func createExtension(_ context.Context, set extension.CreateSettings, cfg component.Config) (extension.Extension, error) {
	return newStorageExt(cfg.(*Config), set.TelemetrySettings), nil
}
