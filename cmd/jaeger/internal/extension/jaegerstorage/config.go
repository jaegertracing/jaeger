// Copyright (c) 2023 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerstorage

import (
	"go.opentelemetry.io/collector/confmap"
	"go.opentelemetry.io/collector/confmap/xconfmap"

	"github.com/jaegertracing/jaeger/cmd/internal/storageconfig"
	"github.com/jaegertracing/jaeger/internal/config/promcfg"
	escfg "github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
	"github.com/jaegertracing/jaeger/internal/storage/metricstore/prometheus"
	es "github.com/jaegertracing/jaeger/internal/storage/v1/elasticsearch"
)

var (
	_ xconfmap.Validator  = (*Config)(nil)
	_ confmap.Unmarshaler = (*MetricBackend)(nil)
)

// Config contains the storage configuration for jaegerstorage extension.
// It uses the same TraceBackends structure as cmd/internal/storageconfig but adds
// jaeger-specific MetricBackends for Prometheus support.
type Config struct {
	TraceBackends  map[string]storageconfig.TraceBackend `mapstructure:"backends"`
	MetricBackends map[string]MetricBackend              `mapstructure:"metric_backends"`
}

// PrometheusConfiguration extends the shared config with prometheus-specific configuration.
type PrometheusConfiguration struct {
	Configuration  promcfg.Configuration `mapstructure:",squash"`
	Authentication escfg.Authentication  `mapstructure:"auth"`
}

// MetricBackend contains configuration for a single metric storage backend.
// This adds Prometheus support on top of the shared MetricBackend.
type MetricBackend struct {
	Prometheus    *PrometheusConfiguration `mapstructure:"prometheus"`
	Elasticsearch *escfg.Configuration     `mapstructure:"elasticsearch"`
	Opensearch    *escfg.Configuration     `mapstructure:"opensearch"`
}

func (cfg *Config) Validate() error {
	// Delegate to shared validation logic
	sharedCfg := storageconfig.Config{
		TraceBackends: cfg.TraceBackends,
	}
	return sharedCfg.Validate()
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
