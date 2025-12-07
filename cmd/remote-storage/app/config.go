// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"fmt"

	"github.com/spf13/viper"
	"go.opentelemetry.io/collector/config/configgrpc"
	"go.opentelemetry.io/collector/config/confignet"

	"github.com/jaegertracing/jaeger/cmd/internal/storageconfig"
	"github.com/jaegertracing/jaeger/internal/storage/v2/memory"
	"github.com/jaegertracing/jaeger/internal/tenancy"
)

// Config represents the configuration for remote-storage service.
type Config struct {
	GRPC    configgrpc.ServerConfig `mapstructure:"grpc"`
	Tenancy tenancy.Options         `mapstructure:"multi_tenancy"`
	// This configuration is the same as of the main `jaeger` binary,
	// but only one backend should be defined.
	Storage storageconfig.Config `mapstructure:"storage"`
}

// LoadConfigFromViper loads the configuration from Viper.
func LoadConfigFromViper(v *viper.Viper) (*Config, error) {
	cfg := &Config{}

	// Unmarshal the entire configuration
	if err := v.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal configuration: %w", err)
	}

	// Validate storage configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return cfg, nil
}

// Validate validates the configuration.
func (c *Config) Validate() error {
	// Validate storage configuration
	if err := c.Storage.Validate(); err != nil {
		return err
	}

	// Ensure only one backend is defined for remote-storage
	if len(c.Storage.TraceBackends) > 1 {
		return fmt.Errorf("remote-storage only supports a single storage backend, but %d were configured", len(c.Storage.TraceBackends))
	}

	return nil
}

// GetStorageName returns the name of the first configured storage backend.
// This is used as the default storage when not otherwise specified.
func (c *Config) GetStorageName() string {
	for name := range c.Storage.TraceBackends {
		return name
	}
	return ""
}

// DefaultConfig returns a default configuration with memory storage.
// This is used when no configuration file is provided.
func DefaultConfig() *Config {
	return &Config{
		GRPC: configgrpc.ServerConfig{
			NetAddr: confignet.AddrConfig{
				Endpoint:  ":17271",
				Transport: confignet.TransportTypeTCP,
			},
		},
		Storage: storageconfig.Config{
			TraceBackends: map[string]storageconfig.TraceBackend{
				"memory": {
					Memory: &memory.Configuration{
						MaxTraces: 1_000_000,
					},
				},
			},
		},
	}
}
