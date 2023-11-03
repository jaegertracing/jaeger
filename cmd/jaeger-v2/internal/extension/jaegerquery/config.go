// Copyright (c) 2023 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerquery

import (
	"github.com/asaskevich/govalidator"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/confighttp"

	queryApp "github.com/jaegertracing/jaeger/cmd/query/app"
	"github.com/jaegertracing/jaeger/pkg/tenancy"
)

var _ component.ConfigValidator = (*Config)(nil)

// Config represents the configuration for jaeger-query,
type Config struct {
	queryApp.QueryOptionsBase `mapstructure:",squash"`

	TraceStoragePrimary           string `valid:"required" mapstructure:"trace_storage"`
	TraceStorageArchive           string `valid:"optional" mapstructure:"trace_storage_archive"`
	confighttp.HTTPServerSettings `mapstructure:",squash"`
	Tenancy                       tenancy.Options `mapstructure:"multi_tenancy"`
}

func (cfg *Config) Validate() error {
	_, err := govalidator.ValidateStruct(cfg)
	return err
}
