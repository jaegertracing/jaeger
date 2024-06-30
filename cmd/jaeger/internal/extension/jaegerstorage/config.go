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
)

// Config has the configuration for jaeger-query,
type Config struct {
	Memory        map[string]memory.Configuration   `mapstructure:"memory"`
	Badger        map[string]badger.NamespaceConfig `mapstructure:"badger"`
	GRPC          map[string]grpc.ConfigV2          `mapstructure:"grpc"`
	Opensearch    map[string]esCfg.Configuration    `mapstructure:"opensearch"`
	Elasticsearch map[string]esCfg.Configuration    `mapstructure:"elasticsearch"`
	Cassandra     map[string]cassandra.Options      `mapstructure:"cassandra"`
	// TODO add other storage types here
	// TODO how will this work with 3rd party storage implementations?
	//      Option: instead of looking for specific name, check interface.
}

func (cfg *Config) Validate() error {
	emptyCfg := createDefaultConfig().(*Config)
	if reflect.DeepEqual(*cfg, *emptyCfg) {
		return fmt.Errorf("%s: no storage type present in config", ID)
	}
	return nil
}
