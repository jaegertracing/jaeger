// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package clickhouse

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/open-telemetry/opentelemetry-collector-contrib/extension/basicauthextension"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/config/configoptional"
	"go.opentelemetry.io/collector/config/configtls"

	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/clickhousetest"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/sql"
	"github.com/jaegertracing/jaeger/internal/telemetry"
)

func TestFactory(t *testing.T) {
	tests := []struct {
		name         string
		createSchema bool
	}{
		{
			name:         "without schema creation",
			createSchema: false,
		},
		{
			name:         "with schema creation",
			createSchema: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := clickhousetest.NewServer(clickhousetest.FailureConfig{})
			defer srv.Close()

			cfg := Configuration{
				Protocol: "http",
				Addresses: []string{
					srv.Listener.Addr().String(),
				},
				Database: "default",
				Auth: Authentication{
					Basic: configoptional.Some(basicauthextension.ClientAuthSettings{
						Username: "user",
						Password: "password",
					}),
				},
			}

			f, err := NewFactory(context.Background(), cfg, telemetry.NoopSettings())
			require.NoError(t, err)
			require.NotNil(t, f)

			tr, err := f.CreateTraceReader()
			require.NoError(t, err)
			require.NotNil(t, tr)

			tw, err := f.CreateTraceWriter()
			require.NoError(t, err)
			require.NotNil(t, tw)

			dr, err := f.CreateDependencyReader()
			require.NoError(t, err)
			require.NotNil(t, dr)

			mr, err := f.CreateMetricsReader()
			require.NoError(t, err)
			require.NotNil(t, mr)

			err = f.Purge(context.Background())
			require.NoError(t, err)

			require.NoError(t, f.Close())
		})
	}
}

func defaultSchemaParams(ttlSeconds int64) schemaTemplateParams {
	return schemaTemplateParams{
		TTLSeconds:                      ttlSeconds,
		TraceIDBloomFilterFalsePositive: "0.0250",
	}
}

func TestNewFactory_Errors(t *testing.T) {
	createSpansTableQuery, err := loadTemplate("test", sql.CreateSpansTable, defaultSchemaParams(0))
	require.NoError(t, err)

	createTraceIDTsTableQuery, err := loadTemplate("test_ts", sql.CreateTraceIDTimestampsTable, defaultSchemaParams(0))
	require.NoError(t, err)

	tests := []struct {
		name          string
		failureConfig clickhousetest.FailureConfig
		expectedError string
	}{
		{
			name: "ping error",
			failureConfig: clickhousetest.FailureConfig{
				clickhousetest.PingQuery: assert.AnError,
			},
			expectedError: "failed to ping ClickHouse",
		},
		{
			name: "spans table creation error",
			failureConfig: clickhousetest.FailureConfig{
				createSpansTableQuery: assert.AnError,
			},
			expectedError: "failed to create spans table",
		},
		{
			name: "services table creation error",
			failureConfig: clickhousetest.FailureConfig{
				sql.CreateServicesTable: assert.AnError,
			},
			expectedError: "failed to create services table",
		},
		{
			name: "services materialized view creation error",
			failureConfig: clickhousetest.FailureConfig{
				sql.CreateServicesMaterializedView: assert.AnError,
			},
			expectedError: "failed to create services materialized view",
		},
		{
			name: "operations table creation error",
			failureConfig: clickhousetest.FailureConfig{
				sql.CreateOperationsTable: assert.AnError,
			},
			expectedError: "failed to create operations table",
		},
		{
			name: "operations materialized view creation error",
			failureConfig: clickhousetest.FailureConfig{
				sql.CreateOperationsMaterializedView: assert.AnError,
			},
			expectedError: "failed to create operations materialized view",
		},
		{
			name: "trace id timestamps table creation error",
			failureConfig: clickhousetest.FailureConfig{
				createTraceIDTsTableQuery: assert.AnError,
			},
			expectedError: "failed to create trace id timestamps table",
		},
		{
			name: "trace id timestamps materialized view creation error",
			failureConfig: clickhousetest.FailureConfig{
				sql.CreateTraceIDTimestampsMaterializedView: assert.AnError,
			},
			expectedError: "failed to create trace id timestamps materialized view",
		},
		{
			name: "attribute metadata table creation error",
			failureConfig: clickhousetest.FailureConfig{
				sql.CreateAttributeMetadataTable: assert.AnError,
			},
			expectedError: "failed to create attribute metadata table",
		},
		{
			name: "attribute metadata materialized view creation error",
			failureConfig: clickhousetest.FailureConfig{
				sql.CreateAttributeMetadataMaterializedView: assert.AnError,
			},
			expectedError: "failed to create attribute metadata materialized view",
		},
		{
			name: "event attribute metadata materialized view creation error",
			failureConfig: clickhousetest.FailureConfig{
				sql.CreateEventAttributeMetadataMaterializedView: assert.AnError,
			},
			expectedError: "failed to create event attribute metadata materialized view",
		},
		{
			name: "link attribute metadata materialized view creation error",
			failureConfig: clickhousetest.FailureConfig{
				sql.CreateLinkAttributeMetadataMaterializedView: assert.AnError,
			},
			expectedError: "failed to create link attribute metadata materialized view",
		},
		{
			name: "dependencies table creation error",
			failureConfig: clickhousetest.FailureConfig{
				sql.CreateDependenciesTable: assert.AnError,
			},
			expectedError: "failed to create dependencies table",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := clickhousetest.NewServer(tt.failureConfig)
			defer srv.Close()

			cfg := Configuration{
				Protocol: "http",
				Addresses: []string{
					srv.Listener.Addr().String(),
				},
				DialTimeout:  1 * time.Second,
				CreateSchema: true,
			}

			f, err := NewFactory(context.Background(), cfg, telemetry.NoopSettings())
			require.ErrorContains(t, err, tt.expectedError)
			require.Nil(t, f)
		})
	}
}

func TestPurge(t *testing.T) {
	tests := []struct {
		name          string
		failureConfig clickhousetest.FailureConfig
		expectedError string
	}{
		{
			name: "truncate spans table error",
			failureConfig: clickhousetest.FailureConfig{
				sql.TruncateSpans: assert.AnError,
			},
			expectedError: "failed to purge spans",
		},
		{
			name: "truncate services table error",
			failureConfig: clickhousetest.FailureConfig{
				sql.TruncateServices: assert.AnError,
			},
			expectedError: "failed to purge services",
		},
		{
			name: "truncate operations table error",
			failureConfig: clickhousetest.FailureConfig{
				sql.TruncateOperations: assert.AnError,
			},
			expectedError: "failed to purge operations",
		},
		{
			name: "truncate trace_id_timestamps table error",
			failureConfig: clickhousetest.FailureConfig{
				sql.TruncateTraceIDTimestamps: assert.AnError,
			},
			expectedError: "failed to purge trace_id_timestamps",
		},
		{
			name: "truncate attribute_metadata table error",
			failureConfig: clickhousetest.FailureConfig{
				sql.TruncateAttributeMetadata: assert.AnError,
			},
			expectedError: "failed to purge attribute_metadata",
		},
		{
			name: "truncate dependencies table error",
			failureConfig: clickhousetest.FailureConfig{
				sql.TruncateDependencies: assert.AnError,
			},
			expectedError: "failed to purge dependencies",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := clickhousetest.NewServer(tt.failureConfig)
			defer srv.Close()

			cfg := Configuration{
				Protocol: "http",
				Addresses: []string{
					srv.Listener.Addr().String(),
				},
				DialTimeout:  1 * time.Second,
				CreateSchema: true,
			}

			f, err := NewFactory(context.Background(), cfg, telemetry.NoopSettings())
			require.NoError(t, err)
			t.Cleanup(func() {
				require.NoError(t, f.Close())
			})

			err = f.Purge(context.Background())
			require.ErrorContains(t, err, tt.expectedError)
		})
	}
}

func TestGetProtocol(t *testing.T) {
	tests := []struct {
		protocol string
		expected clickhouse.Protocol
	}{
		{
			protocol: "http",
			expected: clickhouse.HTTP,
		},
		{
			protocol: "native",
			expected: clickhouse.Native,
		},
		{
			protocol: "",
			expected: clickhouse.Native,
		},
		{
			protocol: "unknown",
			expected: clickhouse.Native,
		},
	}

	for _, tt := range tests {
		t.Run(tt.protocol, func(t *testing.T) {
			result := getProtocol(tt.protocol)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestNewFactory_TLSLoadError(t *testing.T) {
	cfg := Configuration{
		Protocol:  "native",
		Addresses: []string{"localhost:9440"},
		TLS: configoptional.Some(configtls.ClientConfig{
			Config: configtls.Config{
				CAFile: "/nonexistent/ca.pem",
			},
		}),
	}
	f, err := NewFactory(context.Background(), cfg, telemetry.NoopSettings())
	require.ErrorContains(t, err, "failed to load TLS configuration")
	require.Nil(t, f)
}

func TestNewFactory_TLSLoadSuccess(t *testing.T) {
	srv := clickhousetest.NewServer(clickhousetest.FailureConfig{})
	defer srv.Close()
	cfg := Configuration{
		Protocol:  "native",
		Addresses: []string{srv.Listener.Addr().String()},
		TLS: configoptional.Some(configtls.ClientConfig{
			InsecureSkipVerify: true,
		}),
	}
	// TLS config loads successfully; connection fails because the test server is plain (no TLS).
	_, err := NewFactory(context.Background(), cfg, telemetry.NoopSettings())
	require.Error(t, err)
	require.NotContains(t, err.Error(), "failed to load TLS configuration")
}

func TestNewSchemaBuilder_Errors(t *testing.T) {
	originalLoadTemplate := loadTemplate
	t.Cleanup(func() { loadTemplate = originalLoadTemplate })

	tests := []struct {
		name          string
		mockFn        func(name, tmplBody string, data any) (string, error)
		expectedError string
	}{
		{
			name: "first loadTemplate call fails",
			mockFn: func(_, _ string, _ any) (string, error) {
				return "", errors.New("mock template error")
			},
			expectedError: "mock template error",
		},
		{
			name: "second loadTemplate call fails",
			mockFn: func() func(name, tmplBody string, data any) (string, error) {
				calls := 0
				return func(name, tmplBody string, data any) (string, error) {
					calls++
					if calls >= 2 {
						return "", errors.New("mock template error")
					}
					return loadTemplateImpl(name, tmplBody, data)
				}
			}(),
			expectedError: "mock template error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loadTemplate = tt.mockFn
			_, err := NewFactory(
				context.Background(),
				Configuration{CreateSchema: true},
				telemetry.NoopSettings(),
			)
			require.ErrorContains(t, err, tt.expectedError)
		})
	}
}

func TestLoadTemplate(t *testing.T) {
	tests := []struct {
		name     string
		tmplBody string
		data     any
		expected string
		errorMsg string
	}{
		{
			name:     "valid template",
			tmplBody: "Hello {{ .Name }}",
			data:     struct{ Name string }{Name: "Jaeger"},
			expected: "Hello Jaeger",
		},
		{
			name:     "parse error",
			tmplBody: "{{ bad syntax",
			data:     nil,
			errorMsg: "failed to parse",
		},
		{
			name:     "execution error",
			tmplBody: "Hello {{ .Name.Invalid }}",
			data:     struct{ Name string }{Name: "Jaeger"},
			errorMsg: "failed to execute",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := loadTemplate("test_tmpl", tt.tmplBody, tt.data)
			if tt.errorMsg != "" {
				require.ErrorContains(t, err, tt.errorMsg)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, res)
			}
		})
	}
}

func TestCreateSpansTableTemplate(t *testing.T) {
	t.Run("without TTL", func(t *testing.T) {
		queryWithoutTTL, err := loadTemplate("test_no_ttl", sql.CreateSpansTable, defaultSchemaParams(0))
		require.NoError(t, err)
		assert.NotContains(t, queryWithoutTTL, "TTL start_time")
		assert.Contains(t, queryWithoutTTL, "bloom_filter(0.0250)")
	})

	t.Run("with TTL", func(t *testing.T) {
		queryWithTTL, err := loadTemplate("test_ttl", sql.CreateSpansTable, defaultSchemaParams(86400))
		require.NoError(t, err)
		assert.Contains(t, queryWithTTL, "TTL start_time + INTERVAL 86400 SECOND DELETE")
	})

	t.Run("with custom bloom filter false positive", func(t *testing.T) {
		params := schemaTemplateParams{
			TTLSeconds:                      0,
			TraceIDBloomFilterFalsePositive: "0.0001",
		}
		query, err := loadTemplate("test_bloom", sql.CreateSpansTable, params)
		require.NoError(t, err)
		assert.Contains(t, query, "bloom_filter(0.0001)")
	})
}

func TestCreateTraceIDTimestampsTableTemplate(t *testing.T) {
	t.Run("without TTL", func(t *testing.T) {
		queryWithoutTTL, err := loadTemplate("test_no_ttl_trace", sql.CreateTraceIDTimestampsTable, defaultSchemaParams(0))
		require.NoError(t, err)
		assert.NotContains(t, queryWithoutTTL, "TTL end")
	})

	t.Run("with TTL", func(t *testing.T) {
		queryWithTTL, err := loadTemplate("test_ttl_trace", sql.CreateTraceIDTimestampsTable, defaultSchemaParams(86400))
		require.NoError(t, err)
		assert.Contains(t, queryWithTTL, "TTL end + INTERVAL 86400 SECOND DELETE")
	})
}

func TestSchemaParams(t *testing.T) {
	t.Run("default bloom filter", func(t *testing.T) {
		params := schemaParams(Configuration{})
		assert.Equal(t, "0.0250", params.TraceIDBloomFilterFalsePositive)
		assert.Equal(t, int64(0), params.TTLSeconds)
	})

	t.Run("custom bloom filter", func(t *testing.T) {
		fp := 0.0001
		params := schemaParams(Configuration{TraceIDBloomFilterFalsePositive: &fp})
		assert.Equal(t, "0.0001", params.TraceIDBloomFilterFalsePositive)
	})
}
