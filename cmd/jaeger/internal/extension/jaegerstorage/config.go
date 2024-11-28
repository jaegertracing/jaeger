// Copyright (c) 2023 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerstorage

import (
	"errors"
	"fmt"
	"reflect"
	"time"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/confmap"

	casCfg "github.com/jaegertracing/jaeger/pkg/cassandra/config"
	esCfg "github.com/jaegertracing/jaeger/pkg/es/config"
	promCfg "github.com/jaegertracing/jaeger/pkg/prometheus/config"
	"github.com/jaegertracing/jaeger/plugin/metrics/prometheus"
	"github.com/jaegertracing/jaeger/plugin/storage/badger"
	"github.com/jaegertracing/jaeger/plugin/storage/cassandra"
	"github.com/jaegertracing/jaeger/plugin/storage/es"
	"github.com/jaegertracing/jaeger/plugin/storage/grpc"
	"github.com/jaegertracing/jaeger/plugin/storage/memory"
)

var (
	_ component.ConfigValidator = (*Config)(nil)
	_ confmap.Unmarshaler       = (*TraceBackend)(nil)
	_ confmap.Unmarshaler       = (*MetricBackend)(nil)
)

// Config contains configuration(s) for jaeger trace storage.
// Keys in the map are storage names that can be used to refer to them
// from other components, e.g. from jaeger_storage_exporter or jaeger_query.
// We tried to alias this type directly to a map, but conf did not populated it correctly.
// Note also that the Backend struct has a custom unmarshaler.
type Config struct {
	TraceBackends  map[string]TraceBackend  `mapstructure:"backends"`
	MetricBackends map[string]MetricBackend `mapstructure:"metric_backends"`
	Roles          Roles                    `mapstructure:"roles"`
}

// TraceBackend contains configuration for a single trace storage backend.
type TraceBackend struct {
	Memory        *memory.Configuration `mapstructure:"memory"`
	Badger        *badger.Config        `mapstructure:"badger"`
	GRPC          *grpc.Config          `mapstructure:"grpc"`
	Cassandra     *cassandra.Options    `mapstructure:"cassandra"`
	Elasticsearch *esCfg.Configuration  `mapstructure:"elasticsearch"`
	Opensearch    *esCfg.Configuration  `mapstructure:"opensearch"`
}

// MetricBackend contains configuration for a single metric storage backend.
type MetricBackend struct {
	Prometheus *promCfg.Configuration `mapstructure:"prometheus"`
}

type Roles struct {
	Traces        string `mapstructure:"traces"`
	TracesArchive string `mapstructure:"traces_archive"`
	Metrics       string `mapstructure:"metrics"`
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
			Primary: cassandra.NamespaceConfig{
				Configuration: casCfg.DefaultConfiguration(),
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
		cfg.Prometheus = &v
	}
	return conf.Unmarshal(cfg)
}
