// Copyright (c) 2023 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerstorage

import (
	"go.opentelemetry.io/collector/confmap"

	"github.com/jaegertracing/jaeger/cmd/internal/storageconfig"
)

var _ confmap.Validator = (*Config)(nil)

// Config embeds the shared storage configuration.
type Config struct {
	storageconfig.Config `mapstructure:",squash"`
}

func (cfg *Config) Validate() error {
	// Delegate to shared validation logic
	return cfg.Config.Validate()
}
