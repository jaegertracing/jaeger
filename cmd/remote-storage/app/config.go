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
	"github.com/jaegertracing/jaeger/internal/jconfmap"
	"github.com/jaegertracing/jaeger/internal/storage/v2/memory"
	"github.com/jaegertracing/jaeger/internal/tenancy"
	"github.com/jaegertracing/jaeger/ports"
)

// Config is the whole configuration file of the remote-storage service.
type Config struct {
	// Service holds the admin server, logging and metrics sections.
	Service ServiceConfig           `mapstructure:",squash"`
	GRPC    configgrpc.ServerConfig `mapstructure:"grpc"`
	Tenancy tenancy.Options         `mapstructure:"multi_tenancy"`
	// This configuration is the same as of the main `jaeger` binary,
	// but only one backend should be defined.
	Storage storageconfig.Config `mapstructure:"storage"`
}

// LoadConfigFile reads the configuration file through OpenTelemetry confmap with the same
// providers the main jaeger binary resolves its configuration with. That is what runs the
// backends' Unmarshal hooks, which supply their defaults, decodes configoptional fields,
// expands ${env:VAR} references, rejects unknown keys, and validates every nested section.
func LoadConfigFile(ctx context.Context, path string) (*Config, error) {
	resolver, err := confmap.NewResolver(confmap.ResolverSettings{
		URIs:              []string{"file:" + path},
		ProviderFactories: jconfmap.ProviderFactories(),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create configuration resolver: %w", err)
	}
	conf, err := resolver.Resolve(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to read configuration file %s: %w", path, err)
	}

	// The file is decoded over the defaults, so a section it leaves out keeps them. The
	// storage section starts empty, because a backend the file names would otherwise sit
	// beside the default one.
	cfg := defaultConfigWithoutStorage()
	if err := conf.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal configuration: %w", err)
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

func defaultConfigWithoutStorage() *Config {
	return &Config{
		Service: DefaultServiceConfig(ports.RemoteStorageAdminHTTP),
		GRPC: configgrpc.ServerConfig{
			NetAddr: confignet.AddrConfig{
				Endpoint:  ports.PortToHostPort(ports.RemoteStorageGRPC),
				Transport: confignet.TransportTypeTCP,
			},
		},
	}
}

// DefaultConfig returns a default configuration with memory storage.
// This is used when no configuration file is provided.
func DefaultConfig() *Config {
	cfg := defaultConfigWithoutStorage()
	cfg.Storage = storageconfig.Config{
		TraceBackends: map[string]storageconfig.TraceBackend{
			"memory": {
				Memory: &memory.Configuration{
					MaxTraces: 1_000_000,
				},
			},
		},
	}
	return cfg
}
