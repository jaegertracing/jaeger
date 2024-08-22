// Copyright (c) 2019 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package grpc

import (
	"time"

	"go.opentelemetry.io/collector/config/configgrpc"
	"go.opentelemetry.io/collector/exporter/exporterhelper"

	"github.com/jaegertracing/jaeger/pkg/config/tlscfg"
	"github.com/jaegertracing/jaeger/pkg/tenancy"
	"github.com/jaegertracing/jaeger/plugin/storage/grpc/shared"
)

// Configuration describes the options to customize the storage behavior.
type Configuration struct {
	RemoteServerAddr     string `yaml:"server" mapstructure:"server"`
	RemoteTLS            tlscfg.Options
	RemoteConnectTimeout time.Duration `yaml:"connection-timeout" mapstructure:"connection-timeout"`
	TenancyOpts          tenancy.Options
}

type ConfigV2 struct {
	Tenancy                        tenancy.Options `mapstructure:"multi_tenancy"`
	configgrpc.ClientConfig        `mapstructure:",squash"`
	exporterhelper.TimeoutSettings `mapstructure:",squash"`
}

func DefaultConfigV2() ConfigV2 {
	return ConfigV2{
		TimeoutSettings: exporterhelper.TimeoutSettings{
			Timeout: defaultConnectionTimeout,
		},
	}
}

func (c *Configuration) TranslateToConfigV2() *ConfigV2 {
	return &ConfigV2{
		ClientConfig: configgrpc.ClientConfig{
			Endpoint:   c.RemoteServerAddr,
			TLSSetting: c.RemoteTLS.ToOtelClientConfig(),
		},
		TimeoutSettings: exporterhelper.TimeoutSettings{
			Timeout: c.RemoteConnectTimeout,
		},
	}
}

// ClientPluginServices defines services plugin can expose and its capabilities
type ClientPluginServices struct {
	shared.PluginServices
	Capabilities shared.PluginCapabilities
}
