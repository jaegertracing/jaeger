// Copyright (c) 2023 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jaegerstorage

import (
	memoryCfg "github.com/jaegertracing/jaeger/pkg/memory/config"
)

// Config has the configuration for jaeger-query,
type Config struct {
	// SpanStorage string                   `mapstructure:"span_storage"`
	// Memory      *memoryCfg.Configuration `mapstructure:"memory"`
	// Memory map[string]MemoryStorage `mapstructure:"memory"`
	Memory map[string]memoryCfg.Configuration `mapstructure:"memory"`
}

type MemoryStorage struct {
	Name string `mapstructure:"name"`
	memoryCfg.Configuration
}

/*
# - jaeger_query needs generic storage, but different kinds
# - a specific backend can support multiple storage APIs
# -

jaeger_query
  span_reader: cassandra_primary
  dependencies_reader: cassandra_primary
  metrics_reader: prometheus
  archive_reader
  archive_writer

jaeger_storage:
  memory:  # defines Factory
  - name: memory
    max_traces: 100000
  cassandra:
  - name: cassandra_primary
    servers: [...]
	namespace: jaeger
  - name: cassandra_archive
    servers: [...]
	namespace: jaeger_archive
*/
