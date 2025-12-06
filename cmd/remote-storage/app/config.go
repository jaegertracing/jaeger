// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"fmt"

	"github.com/spf13/viper"
	"go.opentelemetry.io/collector/config/configgrpc"
	"go.opentelemetry.io/collector/config/confignet"

	"github.com/jaegertracing/jaeger/cmd/internal/storageconfig"
	"github.com/jaegertracing/jaeger/internal/tenancy"
)

// Config represents the configuration for remote-storage service.
type Config struct {
	GRPC    GRPCConfig           `mapstructure:"grpc"`
	Tenancy tenancy.Options      `mapstructure:"multi_tenancy"`
	Storage storageconfig.Config `mapstructure:"storage"`
}

// GRPCConfig holds gRPC server configuration.
type GRPCConfig struct {
	HostPort string `mapstructure:"host-port"`
}

// GetServerOptions returns Options for the server from the configuration.
func (c *Config) GetServerOptions() *Options {
	return &Options{
		ServerConfig: configgrpc.ServerConfig{
			NetAddr: confignet.AddrConfig{
				Endpoint: c.GRPC.HostPort,
			},
		},
		Tenancy: c.Tenancy,
	}
}

// LoadConfigFromViper loads the configuration from Viper.
func LoadConfigFromViper(v *viper.Viper) (*Config, error) {
	cfg := &Config{}

	// Unmarshal the entire configuration
	if err := v.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal configuration: %w", err)
	}

	// Validate storage configuration
	if err := cfg.Storage.Validate(); err != nil {
		return nil, fmt.Errorf("invalid storage configuration: %w", err)
	}

	return cfg, nil
}

// GetStorageName returns the name of the first configured storage backend.
// This is used as the default storage when not otherwise specified.
func (c *Config) GetStorageName() string {
	for name := range c.Storage.TraceBackends {
		return name
	}
	return ""
}
