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

// ResolverSettings returns the confmap resolver settings every Jaeger binary reads its
// configuration with, so that a file is read the same way whichever binary loads it: the
// same `${scheme:...}` providers, and `${VAR}` with no scheme read from the environment,
// as the OpenTelemetry Collector does. The caller adds the URIs to read.
func ResolverSettings() confmap.ResolverSettings {
	return confmap.ResolverSettings{
		ProviderFactories: []confmap.ProviderFactory{
			envprovider.NewFactory(),
			fileprovider.NewFactory(),
			httpprovider.NewFactory(),
			httpsprovider.NewFactory(),
			yamlprovider.NewFactory(),
		},
		DefaultScheme: "env",
	}
}
