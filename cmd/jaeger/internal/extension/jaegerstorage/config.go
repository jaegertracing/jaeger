// Copyright (c) 2023 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerstorage

import (
	"fmt"
	"reflect"

	cassandraCfg "github.com/jaegertracing/jaeger/pkg/cassandra/config"
	esCfg "github.com/jaegertracing/jaeger/pkg/es/config"
	memoryCfg "github.com/jaegertracing/jaeger/pkg/memory/config"
	badgerCfg "github.com/jaegertracing/jaeger/plugin/storage/badger"
	grpcCfg "github.com/jaegertracing/jaeger/plugin/storage/grpc/config"
)

// Config has the configuration for jaeger-query,
type Config struct {
	Memory        map[string]memoryCfg.Configuration    `mapstructure:"memory"`
	Badger        map[string]badgerCfg.NamespaceConfig  `mapstructure:"badger"`
	GRPC          map[string]grpcCfg.Configuration      `mapstructure:"grpc"`
	Opensearch    map[string]esCfg.Configuration        `mapstructure:"opensearch"`
	Elasticsearch map[string]esCfg.Configuration        `mapstructure:"elasticsearch"`
	Cassandra     map[string]cassandraCfg.Configuration `mapstructure:"cassandra"`
	// TODO add other storage types here
	// TODO how will this work with 3rd party storage implementations?
	//      Option: instead of looking for specific name, check interface.
}

type MemoryStorage struct {
	Name string `mapstructure:"name"`
	memoryCfg.Configuration
}

func (cfg *Config) Validate() error {
	emptyCfg := createDefaultConfig().(*Config)
	//nolint:govet // The remoteRPCClient field in GRPC.Configuration contains error type
	if reflect.DeepEqual(*cfg, *emptyCfg) {
		return fmt.Errorf("%s: no storage type present in config", ID)
	} else {
		return nil
	}
}
