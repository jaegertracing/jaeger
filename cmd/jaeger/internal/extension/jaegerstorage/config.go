// Copyright (c) 2023 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerstorage

import (
	"fmt"
	"reflect"

	esCfg "github.com/jaegertracing/jaeger/pkg/es/config"
	"github.com/jaegertracing/jaeger/plugin/storage/badger"
	"github.com/jaegertracing/jaeger/plugin/storage/cassandra"
	"github.com/jaegertracing/jaeger/plugin/storage/grpc"
	"github.com/jaegertracing/jaeger/plugin/storage/memory"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/confmap"
)

var (
	_ component.ConfigValidator = (*Config)(nil)
	// _ confmap.Unmarshaler       = (*Config)(nil)
)

// Config contains configuration(s) for jaeger trace storage.
// Keys in the map are storage names that can be used to refer to them
// from other components, e.g. from jaeger_storage_exporter or jaeger_query.
type Config struct {
	Backends map[string]Backend `mapstructure:"backends"`
}

type Backend struct {
	Memory        *memory.Configuration   `mapstructure:"memory"`
	Badger        *badger.NamespaceConfig `mapstructure:"badger"`
	GRPC          *grpc.ConfigV2          `mapstructure:"grpc"`
	Cassandra     *cassandra.Options      `mapstructure:"cassandra"`
	Elasticsearch *esCfg.Configuration    `mapstructure:"elasticsearch"`
	Opensearch    *esCfg.Configuration    `mapstructure:"opensearch"`
}

func (cfg *Backend) Unmarshal(conf *confmap.Conf) error {
	// apply defaults
	if conf.IsSet("memory") {
		cfg.Memory = &memory.Configuration{
			MaxTraces: 1_000_000,
		}
	}
	if conf.IsSet("badger") {
		v := badger.DefaultNamespaceConfig()
		cfg.Badger = &v
	}
	if conf.IsSet("grpc") {
		v := grpc.DefaultConfigV2()
		cfg.GRPC = &v
	}
	// TODO add defaults for other storage backends
	return conf.Unmarshal(cfg)
}

func (cfg *Config) Validate() error {
	if len(cfg.Backends) == 0 {
		return fmt.Errorf("at least one storage is required")
	}
	for name, b := range cfg.Backends {
		empty := Backend{}
		if reflect.DeepEqual(b, empty) {
			return fmt.Errorf("no backend defined for storage '%s'", name)
		}
	}
	return nil
}
