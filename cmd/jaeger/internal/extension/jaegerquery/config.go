// Copyright (c) 2023 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerquery

import (
	"time"

	"github.com/asaskevich/govalidator"
	"go.opentelemetry.io/collector/config/configgrpc"
	"go.opentelemetry.io/collector/config/confighttp"
	"go.opentelemetry.io/collector/config/confignet"
	"go.opentelemetry.io/collector/confmap/xconfmap"

	queryapp "github.com/jaegertracing/jaeger/cmd/query/app"
	"github.com/jaegertracing/jaeger/internal/tenancy"
	"github.com/jaegertracing/jaeger/ports"
)

var _ xconfmap.Validator = (*Config)(nil)

// UIConfig contains configuration related to the Jaeger UI.
type UIConfig struct {
	// ConfigFile is the path to a configuration file for the UI.
	ConfigFile string `mapstructure:"config_file" valid:"optional"`
	// AssetsPath is the path for the static assets for the UI (https://github.com/uber/jaeger-ui).
	AssetsPath string `mapstructure:"assets_path" valid:"optional" `
	// LogAccess tells static handler to log access to static assets, useful in debugging.
	LogAccess bool `mapstructure:"log_access" valid:"optional"`
}

// Config represents the configuration for jaeger-query.
type Config struct {
	// Storage holds configuration related to the various data stores that are to be queried.
	Storage Storage `mapstructure:"storage"`
	// BasePath is the base path for all HTTP routes.
	BasePath string `mapstructure:"base_path"`
	// UIConfig contains configuration related to the Jaeger UIConfig.
	UIConfig UIConfig `mapstructure:"ui"`
	// BearerTokenPropagation activate/deactivate bearer token propagation to storage.
	BearerTokenPropagation bool `mapstructure:"bearer_token_propagation"`
	// Tenancy holds the multi-tenancy configuration.
	Tenancy tenancy.Options `mapstructure:"multi_tenancy"`
	// MaxClockSkewAdjust is the maximum duration by which jaeger-query will adjust a span.
	MaxClockSkewAdjust time.Duration `mapstructure:"max_clock_skew_adjust"  valid:"optional"`
	// EnableTracing determines whether traces will be emitted by jaeger-query.
	EnableTracing bool `mapstructure:"enable_tracing"`
	// HTTP holds the HTTP configuration that the query service uses to serve requests.
	HTTP confighttp.ServerConfig `mapstructure:"http"`
	// GRPC holds the GRPC configuration that the query service uses to serve requests.
	GRPC configgrpc.ServerConfig `mapstructure:"grpc"`
}

type Storage struct {
	// TracesPrimary contains the name of the primary trace storage that is being queried.
	TracesPrimary string `mapstructure:"traces" valid:"required"`
	// TracesArchive contains the name of the archive trace storage that is being queried.
	TracesArchive string `mapstructure:"traces_archive" valid:"optional"`
	// Metrics contains the name of the metric storage that is being queried.
	Metrics string `mapstructure:"metrics" valid:"optional"`
}

// DefaultConfig creates the default configuration for jaeger-query.
func DefaultConfig() Config {
	return Config{
		MaxClockSkewAdjust: 0, // disabled by default
		HTTP: confighttp.ServerConfig{
			Endpoint: ports.PortToHostPort(ports.QueryHTTP),
		},
		GRPC: configgrpc.ServerConfig{
			NetAddr: confignet.AddrConfig{
				Endpoint:  ports.PortToHostPort(ports.QueryGRPC),
				Transport: confignet.TransportTypeTCP,
			},
		},
	}
}

func (cfg *Config) Validate() error {
	_, err := govalidator.ValidateStruct(cfg)
	return err
}

// ToQueryOptions converts the Config to the legacy QueryOptions structure
// used by cmd/query/app package.
func (cfg *Config) ToQueryOptions() *queryapp.QueryOptions {
	return &queryapp.QueryOptions{
		BasePath:               cfg.BasePath,
		UIConfig:               cfg.toUIConfig(),
		BearerTokenPropagation: cfg.BearerTokenPropagation,
		Tenancy:                cfg.Tenancy,
		MaxClockSkewAdjust:     cfg.MaxClockSkewAdjust,
		EnableTracing:          cfg.EnableTracing,
		HTTP:                   cfg.HTTP,
		GRPC:                   cfg.GRPC,
	}
}

// toUIConfig converts the local UIConfig to the legacy UIConfig structure
func (cfg *Config) toUIConfig() queryapp.UIConfig {
	return queryapp.UIConfig{
		ConfigFile: cfg.UIConfig.ConfigFile,
		AssetsPath: cfg.UIConfig.AssetsPath,
		LogAccess:  cfg.UIConfig.LogAccess,
	}
}
