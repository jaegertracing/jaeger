// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package remotesampling

import (
	"errors"
	"time"

	"github.com/asaskevich/govalidator"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/configgrpc"
	"go.opentelemetry.io/collector/config/confighttp"
	"go.opentelemetry.io/collector/confmap"
	"go.opentelemetry.io/collector/featuregate"

	"github.com/jaegertracing/jaeger/plugin/sampling/strategyprovider/adaptive"
)

var (
	errNoProvider        = errors.New("no sampling strategy provider specified, expecting 'adaptive' or 'file'")
	errMultipleProviders = errors.New("only one sampling strategy provider can be specified, 'adaptive' or 'file'")
	errNegativeInterval  = errors.New("reload interval must be a positive value, or zero to disable automatic reloading")
)

var (
	_ component.Config          = (*Config)(nil)
	_ component.ConfigValidator = (*Config)(nil)
	_ confmap.Unmarshaler       = (*Config)(nil)

	includeDefaultOpStrategies = featuregate.GlobalRegistry().MustRegister(
		"jaeger.sampling.includeDefaultOpStrategies",
		featuregate.StageBeta, // enabed by default
		featuregate.WithRegisterFromVersion("v2.2.0"),
		featuregate.WithRegisterToVersion("v2.5.0"),
		featuregate.WithRegisterDescription("Forces service strategy to be merged with default strategy, including per-operation overrides."),
		featuregate.WithRegisterReferenceURL("https://github.com/jaegertracing/jaeger/issues/5270"),
	)
)

type Config struct {
	File     *FileConfig              `mapstructure:"file"`
	Adaptive *AdaptiveConfig          `mapstructure:"adaptive"`
	HTTP     *confighttp.ServerConfig `mapstructure:"http"`
	GRPC     *configgrpc.ServerConfig `mapstructure:"grpc"`
}

type FileConfig struct {
	// File specifies a local file as the source of sampling strategies.
	Path string `mapstructure:"path"`
	// ReloadInterval is the time interval to check and reload sampling strategies file
	ReloadInterval time.Duration `mapstructure:"reload_interval"`
}

type AdaptiveConfig struct {
	// SamplingStore is the name of the storage defined in the jaegerstorage extension.
	SamplingStore string `valid:"required" mapstructure:"sampling_store"`

	adaptive.Options `mapstructure:",squash"`
}

// Unmarshal is a custom unmarshaler that allows the factory to provide default values
// for nested configs (like GRPC endpoint) yes still reset the pointers to nil if the
// config did not contain the corresponding sections.
// This is a workaround for the lack of opional fields support in OTEL confmap.
// Issue: https://github.com/open-telemetry/opentelemetry-collector/issues/10266
func (cfg *Config) Unmarshal(conf *confmap.Conf) error {
	// first load the config normally
	err := conf.Unmarshal(cfg)
	if err != nil {
		return err
	}

	// use string names of fields to see if they are set in the confmap
	if !conf.IsSet("file") {
		cfg.File = nil
	}

	if !conf.IsSet("adaptive") {
		cfg.Adaptive = nil
	}

	if !conf.IsSet("grpc") {
		cfg.GRPC = nil
	}

	if !conf.IsSet("http") {
		cfg.HTTP = nil
	}

	return nil
}

func (cfg *Config) Validate() error {
	if cfg.File == nil && cfg.Adaptive == nil {
		return errNoProvider
	}

	if cfg.File != nil && cfg.Adaptive != nil {
		return errMultipleProviders
	}

	if cfg.File != nil && cfg.File.ReloadInterval < 0 {
		return errNegativeInterval
	}

	_, err := govalidator.ValidateStruct(cfg)
	return err
}
