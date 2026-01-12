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
	"go.opentelemetry.io/collector/config/configoptional"
	"go.opentelemetry.io/collector/confmap/xconfmap"
	"go.opentelemetry.io/collector/featuregate"

	"github.com/jaegertracing/jaeger/internal/sampling/samplingstrategy/adaptive"
)

var (
	errNoProvider        = errors.New("no sampling strategy provider specified, expecting 'adaptive' or 'file'")
	errMultipleProviders = errors.New("only one sampling strategy provider can be specified, 'adaptive' or 'file'")
	errNegativeInterval  = errors.New("reload interval must be a positive value, or zero to disable automatic reloading")
)

var (
	_ component.Config   = (*Config)(nil)
	_ xconfmap.Validator = (*Config)(nil)

	_ = featuregate.GlobalRegistry().MustRegister(
		"jaeger.sampling.includeDefaultOpStrategies",
		featuregate.StageStable, // can only be ON
		featuregate.WithRegisterFromVersion("v2.2.0"),
		featuregate.WithRegisterToVersion("v2.5.0"),
		featuregate.WithRegisterDescription("Forces service strategy to be merged with default strategy, including per-operation overrides."),
		featuregate.WithRegisterReferenceURL("https://github.com/jaegertracing/jaeger/issues/5270"),
	)
)

type Config struct {
	File     configoptional.Optional[FileConfig]              `mapstructure:"file"`
	Adaptive configoptional.Optional[AdaptiveConfig]          `mapstructure:"adaptive"`
	HTTP     configoptional.Optional[confighttp.ServerConfig] `mapstructure:"http"`
	GRPC     configoptional.Optional[configgrpc.ServerConfig] `mapstructure:"grpc"`
}

type FileConfig struct {
	// File specifies a local file as the source of sampling strategies.
	Path string `mapstructure:"path"`
	// ReloadInterval is the time interval to check and reload sampling strategies file
	ReloadInterval time.Duration `mapstructure:"reload_interval"`
	// DefaultSamplingProbability is the sampling probability used by the Strategy Store for static sampling
	DefaultSamplingProbability float64 `mapstructure:"default_sampling_probability" valid:"range(0|1)"`
}

type AdaptiveConfig struct {
	// SamplingStore is the name of the storage defined in the jaegerstorage extension.
	SamplingStore string `valid:"required" mapstructure:"sampling_store"`

	adaptive.Options `mapstructure:",squash"`
}

func (cfg *Config) Validate() error {
	if !cfg.File.HasValue() && !cfg.Adaptive.HasValue() {
		return errNoProvider
	}

	if cfg.File.HasValue() && cfg.Adaptive.HasValue() {
		return errMultipleProviders
	}

	if cfg.File.HasValue() {
		fileCfg := cfg.File.Get()
		if fileCfg.ReloadInterval < 0 {
			return errNegativeInterval
		}
		// validate file config fields
		_, err := govalidator.ValidateStruct(fileCfg)
		if err != nil {
			return err
		}
	}

	if cfg.Adaptive.HasValue() {
		// Validate adaptive config fields
		_, err := govalidator.ValidateStruct(cfg.Adaptive.Get())
		if err != nil {
			return err
		}
	}

	return nil
}
