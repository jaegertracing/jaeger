// Copyright (c) 2023 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerstorage

import (
	"context"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/extension"
)

const (
	// ComponentType is the name of this extension in configuration.
	ComponentType = component.Type("jaeger_storage")
)

func NewFactory() extension.Factory {
	return extension.NewFactory(
		ComponentType,
		createDefaultConfig,
		createExtension,
		component.StabilityLevelBeta,
	)
}

func createDefaultConfig() component.Config {
	return &Config{}
}

// createExtension creates the extension based on this config.
func createExtension(_ context.Context, set extension.CreateSettings, cfg component.Config) (extension.Extension, error) {
	return newStorageExt(cfg.(*Config), set.TelemetrySettings), nil
}
