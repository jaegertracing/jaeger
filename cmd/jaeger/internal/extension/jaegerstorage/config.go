// Copyright (c) 2023 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerstorage

import (
	memoryCfg "github.com/jaegertracing/jaeger/pkg/memory/config"
	ch "github.com/jaegertracing/jaeger/plugin/storage/clickhouse"
)

// Config has the configuration for jaeger-query,
type Config struct {
	Memory map[string]memoryCfg.Configuration `mapstructure:"memory"`
	// TODO add other storage types here
	// TODO how will this work with 3rd party storage implementations?
	//      Option: instead of looking for specific name, check interface.

	ClickHouse map[string]ch.Config `mapstructure:"clickhouse"`
}
