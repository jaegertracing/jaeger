// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"fmt"

	"go.opentelemetry.io/collector/config/configgrpc"
	"go.opentelemetry.io/collector/config/confignet"
	"go.opentelemetry.io/collector/confmap"

	"github.com/jaegertracing/jaeger/cmd/internal/storageconfig"
	"github.com/jaegertracing/jaeger/internal/config"
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

// configSections are the top-level keys of the configuration file this service reads. The
// same file also carries the service flags (log level, admin port), which the flags package
// reads through viper, so the loader decodes only these sections and leaves the rest alone.
var configSections = []string{"grpc", "multi_tenancy", "storage"}

// LoadConfigFile reads the configuration file through OpenTelemetry confmap with the same
// providers the main jaeger binary resolves its configuration with. That is what runs the
// backends' Unmarshal hooks, which supply their defaults, decodes configoptional fields,
// expands ${env:VAR} references, rejects unknown keys, and validates every nested section.
func LoadConfigFile(ctx context.Context, path string) (*Config, error) {
	resolver, err := confmap.NewResolver(confmap.ResolverSettings{
		URIs:              []string{"file:" + path},
		ProviderFactories: config.ConfmapProviderFactories(),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create configuration resolver: %w", err)
	}
	conf, err := resolver.Resolve(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to read configuration file %s: %w", path, err)
	}

	// The transport has to be set for the gRPC settings to validate, and the file only ever
	// names the endpoint, so the default goes in before the file is decoded over it.
	cfg := &Config{}
	cfg.GRPC.NetAddr.Transport = confignet.TransportTypeTCP
	targets := map[string]any{
		"grpc":          &cfg.GRPC,
		"multi_tenancy": &cfg.Tenancy,
		"storage":       &cfg.Storage,
	}
	for _, section := range configSections {
		if !conf.IsSet(section) {
			continue
		}
		sub, err := conf.Sub(section)
		if err != nil {
			return nil, fmt.Errorf("failed to read configuration section %q: %w", section, err)
		}
		if err := sub.Unmarshal(targets[section]); err != nil {
			return nil, fmt.Errorf("failed to unmarshal configuration section %q: %w", section, err)
		}
	}

	if err := confmap.Validate(cfg); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}
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
