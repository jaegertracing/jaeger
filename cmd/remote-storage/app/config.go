// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/spf13/viper"
	"go.opentelemetry.io/collector/config/configgrpc"
	"go.opentelemetry.io/collector/config/confignet"
	"go.opentelemetry.io/collector/confmap"

	cascfg "github.com/jaegertracing/jaeger/internal/storage/cassandra/config"
	escfg "github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
	"github.com/jaegertracing/jaeger/internal/storage/v1/badger"
	"github.com/jaegertracing/jaeger/internal/storage/v1/cassandra"
	es "github.com/jaegertracing/jaeger/internal/storage/v1/elasticsearch"
	"github.com/jaegertracing/jaeger/internal/storage/v1/memory"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse"
	"github.com/jaegertracing/jaeger/internal/storage/v2/grpc"
	"github.com/jaegertracing/jaeger/internal/tenancy"
)

// StorageConfig contains configuration for storage backends.
// This is based on jaegerstorage.Config but placed here to avoid internal package imports.
type StorageConfig struct {
	Backends map[string]TraceBackend `mapstructure:"backends"`
}

// TraceBackend contains configuration for a single trace storage backend.
// This mirrors jaegerstorage.TraceBackend.
type TraceBackend struct {
	Memory        *memory.Configuration     `mapstructure:"memory"`
	Badger        *badger.Config            `mapstructure:"badger"`
	GRPC          *grpc.Config              `mapstructure:"grpc"`
	Cassandra     *cassandra.Options        `mapstructure:"cassandra"`
	Elasticsearch *escfg.Configuration      `mapstructure:"elasticsearch"`
	Opensearch    *escfg.Configuration      `mapstructure:"opensearch"`
	ClickHouse    *clickhouse.Configuration `mapstructure:"clickhouse"`
}

// Unmarshal implements confmap.Unmarshaler to provide defaults.
func (cfg *TraceBackend) Unmarshal(conf *confmap.Conf) error {
	// apply defaults
	if conf.IsSet("memory") {
		cfg.Memory = &memory.Configuration{
			MaxTraces: 1_000_000,
		}
	}
	if conf.IsSet("badger") {
		v := badger.DefaultConfig()
		cfg.Badger = v
	}
	if conf.IsSet("grpc") {
		v := grpc.DefaultConfig()
		cfg.GRPC = &v
	}
	if conf.IsSet("cassandra") {
		cfg.Cassandra = &cassandra.Options{
			NamespaceConfig: cassandra.NamespaceConfig{
				Configuration: cascfg.DefaultConfiguration(),
				Enabled:       true,
			},
			SpanStoreWriteCacheTTL: 12 * time.Hour,
			Index: cassandra.IndexConfig{
				Tags:        true,
				ProcessTags: true,
				Logs:        true,
			},
		}
	}
	if conf.IsSet("elasticsearch") {
		v := es.DefaultConfig()
		cfg.Elasticsearch = &v
	}
	if conf.IsSet("opensearch") {
		v := es.DefaultConfig()
		cfg.Opensearch = &v
	}
	return conf.Unmarshal(cfg)
}

// Config represents the configuration for remote-storage service.
type Config struct {
	GRPC    GRPCConfig      `mapstructure:"grpc"`
	Tenancy tenancy.Options `mapstructure:"multi_tenancy"`
	Storage StorageConfig   `mapstructure:"storage"`
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
	if err := cfg.validateStorage(); err != nil {
		return nil, fmt.Errorf("invalid storage configuration: %w", err)
	}

	return cfg, nil
}

// validateStorage validates the storage configuration.
func (c *Config) validateStorage() error {
	if len(c.Storage.Backends) == 0 {
		return errors.New("at least one storage backend is required")
	}
	for name, b := range c.Storage.Backends {
		empty := TraceBackend{}
		if reflect.DeepEqual(b, empty) {
			return fmt.Errorf("empty backend configuration for storage '%s'", name)
		}
	}
	return nil
}

// GetStorageName returns the name of the first configured storage backend.
// This is used as the default storage when not otherwise specified.
func (c *Config) GetStorageName() string {
	for name := range c.Storage.Backends {
		return name
	}
	return ""
}
