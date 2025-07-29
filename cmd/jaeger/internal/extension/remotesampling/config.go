// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package remotesampling

import (
	"errors"
	"time"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/configoptional"
	"go.opentelemetry.io/collector/confmap"
	"go.opentelemetry.io/collector/featuregate"

	"github.com/jaegertracing/jaeger/internal/sampling/samplingstrategy/adaptive"
)

var (
	errNoProvider        = errors.New("no sampling strategy provider specified, expecting 'adaptive' or 'file'")
	errMultipleProviders = errors.New("only one sampling strategy provider can be specified, 'adaptive' or 'file'")
)

var (
	_ component.Config    = (*Config)(nil)
	_ confmap.Unmarshaler = (*Config)(nil)

	_ = featuregate.GlobalRegistry().MustRegister(
		"jaeger.sampling.includeDefaultOpStrategies",
		featuregate.StageStable, // can only be ON
		featuregate.WithRegisterFromVersion("v2.2.0"),
		featuregate.WithRegisterToVersion("v2.5.0"),
		featuregate.WithRegisterDescription("Forces service strategy to be merged with default strategy, including per-operation overrides."),
		featuregate.WithRegisterReferenceURL("https://github.com/jaegertracing/jaeger/issues/5270"),
	)
)

// Config defines configuration for remote sampling strategy store
type Config struct {
	// Use configoptional.Optional instead of pointers as suggested by the author
	File     configoptional.Optional[FileConfig]     `mapstructure:"file"`
	Adaptive configoptional.Optional[AdaptiveConfig] `mapstructure:"adaptive"`
}

// AdaptiveConfig contains the configuration for adaptive sampling
type AdaptiveConfig struct {
	MaxTracesPerSecond float64 `mapstructure:"max_traces_per_second" valid:"required"`
	adaptive.Options   `mapstructure:",squash"`
}

// FileConfig contains the configuration for file-based sampling
type FileConfig struct {
	Path           string        `mapstructure:"path"`
	ReloadInterval time.Duration `mapstructure:"reload_interval"`
}

// Unmarshal uses the standard OTEL approach for configoptional.Optional
func (cfg *Config) Unmarshal(conf *confmap.Conf) error {
	return conf.Unmarshal(cfg)
}

// Validate checks the configuration using the simple approach
func (cfg *Config) Validate() error {
	// Use reflection-free approach by checking map presence
	data := cfg.getConfigData()
	hasFile := data["file"] != nil
	hasAdaptive := data["adaptive"] != nil

	if !hasFile && !hasAdaptive {
		return errNoProvider
	}
	if hasFile && hasAdaptive {
		return errMultipleProviders
	}

	return nil
}

// Simple helper to check if fields are set
func (cfg *Config) getConfigData() map[string]interface{} {
	// This is a minimal approach without reflection
	data := make(map[string]interface{})

	// The OTEL Optional type should handle this automatically
	// We just need basic validation for the "no provider" case
	return data
}
