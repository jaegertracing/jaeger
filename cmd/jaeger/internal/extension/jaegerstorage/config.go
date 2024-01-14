// Copyright (c) 2023 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerstorage

import (
	"fmt"
	"reflect"

	memoryCfg "github.com/jaegertracing/jaeger/pkg/memory/config"
)

// Config has the configuration for jaeger-query,
type Config struct {
	Memory map[string]memoryCfg.Configuration `mapstructure:"memory"`
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
	if reflect.DeepEqual(*cfg, *emptyCfg) {
		return fmt.Errorf("%s: no storage type present in config", ID)
	} else {
		return nil
	}
}
