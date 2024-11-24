// Copyright (c) 2019 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package grpc

import (
	"go.opentelemetry.io/collector/config/configgrpc"
	"go.opentelemetry.io/collector/exporter/exporterhelper"

	"github.com/jaegertracing/jaeger/pkg/tenancy"
	"github.com/jaegertracing/jaeger/plugin/storage/grpc/shared"
)

// Config describes the options to customize the storage behavior
type Config struct {
	Tenancy                      tenancy.Options `mapstructure:"multi_tenancy"`
	configgrpc.ClientConfig      `mapstructure:",squash"`
	exporterhelper.TimeoutConfig `mapstructure:",squash"`
}

func DefaultConfig() Config {
	return Config{
		TimeoutConfig: exporterhelper.TimeoutConfig{
			Timeout: defaultConnectionTimeout,
		},
	}
}

// ClientPluginServices defines services plugin can expose and its capabilities
type ClientPluginServices struct {
	shared.PluginServices
	Capabilities shared.PluginCapabilities
}
