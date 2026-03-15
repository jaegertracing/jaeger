// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package storageconfig

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/confmap"

	escfg "github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
	"github.com/jaegertracing/jaeger/internal/storage/v1/badger"
	"github.com/jaegertracing/jaeger/internal/storage/v2/memory"
)

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name        string
		config      Config
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid config with one backend",
			config: Config{
				TraceBackends: map[string]TraceBackend{
					"memory": {
						Memory: &memory.Configuration{MaxTraces: 10000},
					},
				},
			},
			expectError: false,
		},
		{
			name: "valid config with multiple backends",
			config: Config{
				TraceBackends: map[string]TraceBackend{
					"memory1": {
						Memory: &memory.Configuration{MaxTraces: 10000},
					},
					"memory2": {
						Memory: &memory.Configuration{MaxTraces: 20000},
					},
				},
			},
			expectError: false,
		},
		{
			name: "no backends",
			config: Config{
				TraceBackends: map[string]TraceBackend{},
			},
			expectError: true,
			errorMsg:    "at least one storage backend is required",
		},
		{
			name: "empty backend configuration",
			config: Config{
				TraceBackends: map[string]TraceBackend{
					"empty": {},
				},
			},
			expectError: true,
			errorMsg:    "trace storage 'empty': empty configuration",
		},
		{
			name: "valid metric backend",
			config: Config{
				TraceBackends: map[string]TraceBackend{
					"memory": {Memory: &memory.Configuration{}},
				},
				MetricBackends: map[string]MetricBackend{
					"prometheus": {Prometheus: &PrometheusConfiguration{}},
				},
			},
			expectError: false,
		},
		{
			name: "invalid trace backend",
			config: Config{
				TraceBackends: map[string]TraceBackend{
					"invalid": {
						Memory: &memory.Configuration{},
						Badger: &badger.Config{},
					},
				},
			},
			expectError: true,
			errorMsg:    "trace storage 'invalid': multiple backend types found",
		},
		{
			name: "invalid metric backend",
			config: Config{
				TraceBackends: map[string]TraceBackend{
					"memory": {Memory: &memory.Configuration{}},
				},
				MetricBackends: map[string]MetricBackend{
					"invalid": {
						Prometheus:    &PrometheusConfiguration{},
						Elasticsearch: &escfg.Configuration{},
					},
				},
			},
			expectError: true,
			errorMsg:    "metric storage 'invalid': multiple backend types found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestTraceBackendUnmarshal(t *testing.T) {
	tests := []struct {
		name         string
		configMap    map[string]any
		expectError  bool
		validateFunc func(*testing.T, *TraceBackend)
	}{
		{
			name: "memory backend with defaults",
			configMap: map[string]any{
				"memory": map[string]any{},
			},
			expectError: false,
			validateFunc: func(t *testing.T, tb *TraceBackend) {
				require.NotNil(t, tb.Memory)
				assert.Equal(t, 1_000_000, tb.Memory.MaxTraces)
			},
		},
		{
			name: "memory backend with custom value",
			configMap: map[string]any{
				"memory": map[string]any{
					"max_traces": 50000,
				},
			},
			expectError: false,
			validateFunc: func(t *testing.T, tb *TraceBackend) {
				require.NotNil(t, tb.Memory)
				assert.Equal(t, 50000, tb.Memory.MaxTraces)
			},
		},
		{
			name: "badger backend with defaults",
			configMap: map[string]any{
				"badger": map[string]any{
					"ephemeral": true,
				},
			},
			expectError: false,
			validateFunc: func(t *testing.T, tb *TraceBackend) {
				require.NotNil(t, tb.Badger)
				assert.True(t, tb.Badger.Ephemeral)
			},
		},
		{
			name: "grpc backend with defaults",
			configMap: map[string]any{
				"grpc": map[string]any{
					"endpoint": "localhost:17271",
				},
			},
			expectError: false,
			validateFunc: func(t *testing.T, tb *TraceBackend) {
				require.NotNil(t, tb.GRPC)
				assert.Equal(t, "localhost:17271", tb.GRPC.ClientConfig.Endpoint)
			},
		},
		{
			name: "cassandra backend with defaults",
			configMap: map[string]any{
				"cassandra": map[string]any{},
			},
			expectError: false,
			validateFunc: func(t *testing.T, tb *TraceBackend) {
				require.NotNil(t, tb.Cassandra)
				assert.True(t, tb.Cassandra.Index.Tags)
				assert.True(t, tb.Cassandra.Index.ProcessTags)
				assert.True(t, tb.Cassandra.Index.Logs)
			},
		},
		{
			name: "elasticsearch backend with defaults",
			configMap: map[string]any{
				"elasticsearch": map[string]any{},
			},
			expectError: false,
			validateFunc: func(t *testing.T, tb *TraceBackend) {
				require.NotNil(t, tb.Elasticsearch)
			},
		},
		{
			name: "opensearch backend with defaults",
			configMap: map[string]any{
				"opensearch": map[string]any{},
			},
			expectError: false,
			validateFunc: func(t *testing.T, tb *TraceBackend) {
				require.NotNil(t, tb.Opensearch)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conf := confmap.NewFromStringMap(tt.configMap)
			var tb TraceBackend
			err := tb.Unmarshal(conf)

			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				if tt.validateFunc != nil {
					tt.validateFunc(t, &tb)
				}
			}
		})
	}
}

func TestMetricBackendUnmarshal(t *testing.T) {
	tests := []struct {
		name         string
		configMap    map[string]any
		expectError  bool
		validateFunc func(*testing.T, *MetricBackend)
	}{
		{
			name: "prometheus backend with defaults",
			configMap: map[string]any{
				"prometheus": map[string]any{},
			},
			expectError: false,
			validateFunc: func(t *testing.T, mb *MetricBackend) {
				require.NotNil(t, mb.Prometheus)
			},
		},
		{
			name: "elasticsearch backend",
			configMap: map[string]any{
				"elasticsearch": map[string]any{},
			},
			expectError: false,
			validateFunc: func(t *testing.T, mb *MetricBackend) {
				require.NotNil(t, mb.Elasticsearch)
			},
		},
		{
			name: "opensearch backend",
			configMap: map[string]any{
				"opensearch": map[string]any{},
			},
			expectError: false,
			validateFunc: func(t *testing.T, mb *MetricBackend) {
				require.NotNil(t, mb.Opensearch)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conf := confmap.NewFromStringMap(tt.configMap)
			var mb MetricBackend
			err := mb.Unmarshal(conf)

			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				if tt.validateFunc != nil {
					tt.validateFunc(t, &mb)
				}
			}
		})
	}
}

func getStorageKeys(t reflect.Type) []string {
	var keys []string
	for field := range t.Fields() {
		tag := field.Tag.Get("mapstructure")
		if tag != "" && tag != ",squash" {
			keys = append(keys, tag)
		}
	}
	return keys
}

func TestTraceBackendExclusive(t *testing.T) {
	keys := getStorageKeys(reflect.TypeFor[TraceBackend]())
	for i := range keys {
		for j := i + 1; j < len(keys); j++ {
			key1 := keys[i]
			key2 := keys[j]
			t.Run(fmt.Sprintf("%s+%s", key1, key2), func(t *testing.T) {
				conf := confmap.NewFromStringMap(map[string]any{
					key1: map[string]any{},
					key2: map[string]any{},
				})
				var tb TraceBackend
				err := tb.Unmarshal(conf)
				require.NoError(t, err)

				err = tb.Validate()
				require.Error(t, err)
				assert.Contains(t, err.Error(), "multiple backend types found")
			})
		}
	}
}

func TestMetricBackendExclusive(t *testing.T) {
	keys := getStorageKeys(reflect.TypeFor[MetricBackend]())
	for i := range keys {
		for j := i + 1; j < len(keys); j++ {
			key1 := keys[i]
			key2 := keys[j]
			t.Run(fmt.Sprintf("%s+%s", key1, key2), func(t *testing.T) {
				conf := confmap.NewFromStringMap(map[string]any{
					key1: map[string]any{},
					key2: map[string]any{},
				})
				var mb MetricBackend
				err := mb.Unmarshal(conf)
				require.NoError(t, err)

				err = mb.Validate()
				require.Error(t, err)
				assert.Contains(t, err.Error(), "multiple backend types found")
			})
		}
	}
}
