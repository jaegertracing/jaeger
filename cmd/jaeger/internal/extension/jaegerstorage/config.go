// Copyright (c) 2023 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerstorage

import (
	memoryCfg "github.com/jaegertracing/jaeger/pkg/memory/config"
	badgerCfg "github.com/jaegertracing/jaeger/plugin/storage/badger"
)

// Config has the configuration for jaeger-query,
type Config struct {
	Memory map[string]memoryCfg.Configuration   `mapstructure:"memory"`
	Badger map[string]badgerCfg.NamespaceConfig `mapstructure:"badger"`
	// TODO add other storage types here
	// TODO how will this work with 3rd party storage implementations?
	//      Option: instead of looking for specific name, check interface.
}

type MemoryStorage struct {
	Name string `mapstructure:"name"`
	memoryCfg.Configuration
}

type BadgerStorage struct {
	Name string `mapstructure:"name"`
	badgerCfg.NamespaceConfig
}
