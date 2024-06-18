// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package remotesampling

import (
	"errors"
	"reflect"

	"github.com/asaskevich/govalidator"
	"github.com/jaegertracing/jaeger/plugin/sampling/strategyprovider/adaptive"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/configgrpc"
	"go.opentelemetry.io/collector/config/confighttp"
)

var (
	errNoSource       = errors.New("no sampling strategy provider specified, expecting 'adaptive' or 'file'")
	errMultipleSource = errors.New("only one sampling strategy provider can be specified, 'adaptive' or 'file'")
)

var _ component.ConfigValidator = (*Config)(nil)

type Config struct {
	File     FileConfig              `mapstructure:"file"`
	Adaptive AdaptiveConfig          `mapstructure:"adaptive"`
	HTTP     confighttp.ServerConfig `mapstructure:"http"`
	GRPC     configgrpc.ServerConfig `mapstructure:"grpc"`
}

type FileConfig struct {
	// File specifies a local file as the source of sampling strategies.
	Path string `valid:"required" mapstructure:"path"`
}

type AdaptiveConfig struct {
	// StrategyStore is the name of the strategy storage defined in the jaegerstorage extension.
	StrategyStore string `valid:"required" mapstructure:"strategy_store"`

	adaptive.Options `mapstructure:",squash"`
}

func (cfg *Config) Validate() error {
	emptyCfg := createDefaultConfig().(*Config)
	if reflect.DeepEqual(*cfg, *emptyCfg) {
		return errNoSource
	}

	if cfg.File.Path != "" && cfg.Adaptive.StrategyStore != "" {
		return errMultipleSource
	}
	_, err := govalidator.ValidateStruct(cfg)
	return err
}
