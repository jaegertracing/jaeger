// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

// Package jconfmap holds what every Jaeger binary shares in how it resolves its
// configuration through OpenTelemetry confmap.
package jconfmap

import (
	"go.opentelemetry.io/collector/confmap"
	"go.opentelemetry.io/collector/confmap/provider/envprovider"
	"go.opentelemetry.io/collector/confmap/provider/fileprovider"
	"go.opentelemetry.io/collector/confmap/provider/httpprovider"
	"go.opentelemetry.io/collector/confmap/provider/httpsprovider"
	"go.opentelemetry.io/collector/confmap/provider/yamlprovider"
)

// ProviderFactories returns the confmap providers every Jaeger binary resolves its
// configuration with, so that a configuration file is read the same way, with the same
// `${scheme:...}` references, whichever binary loads it.
func ProviderFactories() []confmap.ProviderFactory {
	return []confmap.ProviderFactory{
		envprovider.NewFactory(),
		fileprovider.NewFactory(),
		httpprovider.NewFactory(),
		httpsprovider.NewFactory(),
		yamlprovider.NewFactory(),
	}
}
