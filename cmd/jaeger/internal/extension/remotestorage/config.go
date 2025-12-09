// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package remotestorage

import (
	"github.com/asaskevich/govalidator"
	"go.opentelemetry.io/collector/config/configgrpc"

	"github.com/jaegertracing/jaeger/internal/tenancy"
)

type Config struct {
	configgrpc.ServerConfig `mapstructure:",squash"`
	Tenancy                 tenancy.Options `mapstructure:"multi_tenancy"`
	Storage                 string          `mapstructure:"storage" valid:"required"`
}

func (cfg *Config) Validate() error {
	_, err := govalidator.ValidateStruct(cfg)
	return err
}
