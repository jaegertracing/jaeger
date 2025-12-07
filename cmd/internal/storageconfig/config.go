// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package storageconfig

import (
	"errors"
	"fmt"
	"reflect"
	"time"

	"go.opentelemetry.io/collector/confmap"

	"github.com/jaegertracing/jaeger/internal/config/promcfg"
	cascfg "github.com/jaegertracing/jaeger/internal/storage/cassandra/config"
	escfg "github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
	"github.com/jaegertracing/jaeger/internal/storage/metricstore/prometheus"
	"github.com/jaegertracing/jaeger/internal/storage/v1/badger"
	"github.com/jaegertracing/jaeger/internal/storage/v1/cassandra"
	es "github.com/jaegertracing/jaeger/internal/storage/v1/elasticsearch"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse"
	"github.com/jaegertracing/jaeger/internal/storage/v2/grpc"
	"github.com/jaegertracing/jaeger/internal/storage/v2/memory"
)

var (
	_ confmap.Unmarshaler = (*TraceBackend)(nil)
	_ confmap.Unmarshaler = (*MetricBackend)(nil)
)

// Config contains configuration(s) for Jaeger trace storage.
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

// MetricBackend contains configuration for a single metric storage backend.
type MetricBackend struct {
	Prometheus    *PrometheusConfiguration `mapstructure:"prometheus"`
	Elasticsearch *escfg.Configuration     `mapstructure:"elasticsearch"`
	Opensearch    *escfg.Configuration     `mapstructure:"opensearch"`
}

type PrometheusConfiguration struct {
	Configuration  promcfg.Configuration `mapstructure:",squash"`
	Authentication escfg.Authentication  `mapstructure:"auth"`
}

// Unmarshal implements confmap.Unmarshaler. This allows us to provide
// defaults for different configs.
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

// Unmarshal implements confmap.Unmarshaler for MetricBackend.
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

// Validate validates the storage configuration.
func (c *Config) Validate() error {
	if len(c.TraceBackends) == 0 {
		return errors.New("at least one storage backend is required")
	}
	for name, b := range c.TraceBackends {
		empty := TraceBackend{}
		if reflect.DeepEqual(b, empty) {
			return fmt.Errorf("empty backend configuration for storage '%s'", name)
		}
	}
	return nil
}
