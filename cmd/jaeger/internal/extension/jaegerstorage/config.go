// Copyright (c) 2023 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerstorage

import (
	"errors"
	"fmt"
	"reflect"
	"time"

	"go.opentelemetry.io/collector/confmap"
	"go.opentelemetry.io/collector/confmap/xconfmap"

	"github.com/jaegertracing/jaeger/internal/config/promcfg"
	cascfg "github.com/jaegertracing/jaeger/internal/storage/cassandra/config"
	escfg "github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
	"github.com/jaegertracing/jaeger/internal/storage/metricstore/prometheus"
	"github.com/jaegertracing/jaeger/internal/storage/v1/badger"
	"github.com/jaegertracing/jaeger/internal/storage/v1/cassandra"
	es "github.com/jaegertracing/jaeger/internal/storage/v1/elasticsearch"
	"github.com/jaegertracing/jaeger/internal/storage/v1/memory"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse"
	"github.com/jaegertracing/jaeger/internal/storage/v2/grpc"
)

var (
	_ xconfmap.Validator  = (*Config)(nil)
	_ confmap.Unmarshaler = (*TraceBackend)(nil)
	_ confmap.Unmarshaler = (*MetricBackend)(nil)
)

// Config contains configuration(s) for jaeger trace storage.
// Keys in the map are storage names that can be used to refer to them
// from other components, e.g. from jaeger_storage_exporter or jaeger_query.
// We tried to alias this type directly to a map, but conf did not populated it correctly.
// Note also that the Backend struct has a custom unmarshaler.
type Config struct {
	TraceBackends  map[string]TraceBackend  `mapstructure:"backends"`
	MetricBackends map[string]MetricBackend `mapstructure:"metric_backends"`
}

// TraceBackend contains configuration for a single trace storage backend.
type TraceBackend struct {
	Memory        *memory.Configuration     `mapstructure:"memory"`
	Badger        *badger.Config            `mapstructure:"badger"`
	GRPC          *grpc.Config              `mapstructure:"grpc"`
	Cassandra     *cassandra.Options        `mapstructure:"cassandra"`
	Elasticsearch *escfg.Configuration      `mapstructure:"elasticsearch"`
	Opensearch    *escfg.Configuration      `mapstructure:"opensearch"`
	ClickHouse    *clickhouse.Configuration `mapstructure:"clickhouse"`
}

// AuthConfig represents authentication configuration for metric backends.
//
// The Authenticator field expects the ID (name) of an HTTP authenticator
// extension that is registered in the running binary and implements
// go.opentelemetry.io/collector/extension/extensionauth.HTTPClient.
//
// Valid values:
//   - "sigv4auth" in the stock Jaeger binary (built-in).
//   - Any other extension name is valid only if that authenticator extension
//     is included in the build; otherwise Jaeger will error at startup when
//     resolving the extension.
//   - Empty/omitted means no auth (default behavior).
type AuthConfig struct {
	// Authenticator is the name (ID) of the HTTP authenticator extension to use.
	Authenticator string `mapstructure:"authenticator"`
}

// PrometheusConfiguration wraps the base Prometheus configuration with auth support.
type PrometheusConfiguration struct {
	promCfg.Configuration `mapstructure:",squash"`
	Auth                  *AuthConfig `mapstructure:"auth,omitempty"`
}

// MetricBackend contains configuration for a single metric storage backend.
type MetricBackend struct {
	Prometheus    *PrometheusConfiguration `mapstructure:"prometheus"`
	Elasticsearch *esCfg.Configuration     `mapstructure:"elasticsearch"`
	Opensearch    *esCfg.Configuration     `mapstructure:"opensearch"`
}

// Unmarshal implements confmap.Unmarshaler. This allows us to provide
// defaults for different configs. It cannot be done in createDefaultConfig()
// because at that time we don't know which backends the user wants to use.
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

func (cfg *Config) Validate() error {
	if len(cfg.TraceBackends) == 0 {
		return errors.New("at least one storage is required")
	}
	for name, b := range cfg.TraceBackends {
		empty := TraceBackend{}
		if reflect.DeepEqual(b, empty) {
			return fmt.Errorf("empty backend configuration for storage '%s'", name)
		}
	}
	return nil
}

func (cfg *MetricBackend) Unmarshal(conf *confmap.Conf) error {
	// apply defaults
	if conf.IsSet("prometheus") {
		v := prometheus.DefaultConfig()
		cfg.Prometheus = &PrometheusConfiguration{
			Configuration: v,
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
