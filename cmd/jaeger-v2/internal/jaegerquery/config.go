// Copyright (c) 2023 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerquery

import (
	"github.com/asaskevich/govalidator"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/confighttp"

	"github.com/jaegertracing/jaeger/pkg/tenancy"
)

var _ component.ConfigValidator = (*Config)(nil)

// Config represents the configuration for jaeger-query,
type Config struct {
	TraceStorage                  string `valid:"required" mapstructure:"trace_storage"`
	confighttp.HTTPServerSettings `mapstructure:",squash"`
	Tenancy                       tenancy.Options `mapstructure:"multi_tenancy"`
}

func (cfg *Config) Validate() error {
	_, err := govalidator.ValidateStruct(cfg)
	return err
}
