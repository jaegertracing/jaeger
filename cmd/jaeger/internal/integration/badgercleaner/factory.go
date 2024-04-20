// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package badgercleaner

import (
	"context"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/extension"
)

// componentType is the name of this extension in configuration.
var componentType = component.MustNewType("badger_cleaner")

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
	return &Config{}
}

func createExtension(_ context.Context, set extension.CreateSettings, cfg component.Config) (extension.Extension, error) {
	return newBadgerCleaner(cfg.(*Config)), nil
}
