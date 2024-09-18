// Copyright (c) 2023 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerquery

import (
	"github.com/asaskevich/govalidator"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/configgrpc"
	"go.opentelemetry.io/collector/config/confighttp"

	queryApp "github.com/jaegertracing/jaeger/cmd/query/app"
	"github.com/jaegertracing/jaeger/pkg/tenancy"
)

var _ component.ConfigValidator = (*Config)(nil)

// Config represents the configuration for jaeger-query,
type Config struct {
	queryApp.QueryOptionsBase `mapstructure:",squash"`
	Connection                Connection `mapstructure:"connection"`
	Storage                   Storage    `mapstructure:"storage"`
}

type Connection struct {
	Tenancy tenancy.Options         `mapstructure:"multi_tenancy"`
	HTTP    confighttp.ServerConfig `mapstructure:"http"`
	GRPC    configgrpc.ServerConfig `mapstructure:"grpc"`
}

type Storage struct {
	TracePrimary string `mapstructure:"trace" valid:"required" `
	TraceArchive string `mapstructure:"trace_archive" valid:"optional"`
	Metric       string `mapstructure:"metric" valid:"optional"`
}

func (cfg *Config) Validate() error {
	_, err := govalidator.ValidateStruct(cfg)
	return err
}
