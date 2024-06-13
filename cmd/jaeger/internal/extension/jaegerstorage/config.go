// Copyright (c) 2023 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerstorage

import (
	"fmt"
	"reflect"

	esCfg "github.com/jaegertracing/jaeger/pkg/es/config"
	memoryCfg "github.com/jaegertracing/jaeger/pkg/memory/config"
	badgerCfg "github.com/jaegertracing/jaeger/plugin/storage/badger"
	"github.com/jaegertracing/jaeger/plugin/storage/cassandra"
	"github.com/jaegertracing/jaeger/plugin/storage/clickhouse"
	grpcCfg "github.com/jaegertracing/jaeger/plugin/storage/grpc"
)

// Config has the configuration for jaeger-query,
type Config struct {
	Memory        map[string]memoryCfg.Configuration   `mapstructure:"memory"`
	Badger        map[string]badgerCfg.NamespaceConfig `mapstructure:"badger"`
	GRPC          map[string]grpcCfg.ConfigV2          `mapstructure:"grpc"`
	Opensearch    map[string]esCfg.Configuration       `mapstructure:"opensearch"`
	Elasticsearch map[string]esCfg.Configuration       `mapstructure:"elasticsearch"`
	Cassandra     map[string]cassandra.Options         `mapstructure:"cassandra"`
	ClickHouse    map[string]clickhouse.Config         `mapstructure:"clickhouse"`
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
