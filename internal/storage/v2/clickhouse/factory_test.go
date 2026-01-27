// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package clickhouse

import (
	"context"
	"testing"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/open-telemetry/opentelemetry-collector-contrib/extension/basicauthextension"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/config/configoptional"

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

			f, err := NewFactory(context.Background(), cfg, telemetry.Settings{})
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

			err = f.Purge(context.Background())
			require.NoError(t, err)

			require.NoError(t, f.Close())
		})
	}
}

func mustExecuteTemplate(t *testing.T, name, templateSQL string, ttlSeconds int64) string {
	result, err := executeTemplate(name, templateSQL, map[string]any{"TTLSeconds": ttlSeconds})
	require.NoError(t, err)
	return result
}

func TestNewFactory_Errors(t *testing.T) {
	// Process templates with TTLSeconds=0 (no TTL) to get the actual SQL that will be sent
	spansTableQuery := mustExecuteTemplate(t, "spans_table", sql.CreateSpansTable, 0)
	traceIDTimestampsQuery := mustExecuteTemplate(t, "trace_id_timestamps_table", sql.CreateTraceIDTimestampsTable, 0)

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
				spansTableQuery: assert.AnError,
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
				traceIDTimestampsQuery: assert.AnError,
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
				DialTimeout: 1 * time.Second,
				Schema: Schema{
					Create: true,
				},
			}

			f, err := NewFactory(context.Background(), cfg, telemetry.Settings{})
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
				DialTimeout: 1 * time.Second,
				Schema: Schema{
					Create: true,
				},
			}

			f, err := NewFactory(context.Background(), cfg, telemetry.Settings{})
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

func TestFactory_TTL(t *testing.T) {
	tests := []struct {
		name         string
		schemaCreate bool
		traceTTL     time.Duration
	}{
		{
			name:         "new database with TTL disabled",
			schemaCreate: true,
			traceTTL:     0,
		},
		{
			name:         "new database with TTL 24 hours",
			schemaCreate: true,
			traceTTL:     24 * time.Hour,
		},
		{
			name:         "new database with TTL 7 days",
			schemaCreate: true,
			traceTTL:     168 * time.Hour,
		},
		{
			name:         "existing database with TTL 24 hours",
			schemaCreate: false,
			traceTTL:     24 * time.Hour,
		},
		{
			name:         "existing database with TTL 7 days",
			schemaCreate: false,
			traceTTL:     168 * time.Hour,
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
				Schema: Schema{
					Create:   tt.schemaCreate,
					TraceTTL: tt.traceTTL,
				},
			}

			f, err := NewFactory(context.Background(), cfg, telemetry.Settings{})
			require.NoError(t, err)
			require.NotNil(t, f)
			require.NoError(t, f.Close())
		})
	}
}

func TestFactory_TTL_Errors(t *testing.T) {
	tests := []struct {
		name          string
		traceTTL      time.Duration
		failureConfig clickhousetest.FailureConfig
		expectedError string
	}{
		{
			name:     "spans TTL application error on existing database",
			traceTTL: 24 * time.Hour,
			failureConfig: clickhousetest.FailureConfig{
				"ALTER TABLE spans MODIFY TTL start_time + INTERVAL 86400 SECOND": assert.AnError,
			},
			expectedError: "failed to apply TTL to spans",
		},
		{
			name:     "trace_id_timestamps TTL application error on existing database",
			traceTTL: 24 * time.Hour,
			failureConfig: clickhousetest.FailureConfig{
				"ALTER TABLE trace_id_timestamps MODIFY TTL start + INTERVAL 86400 SECOND": assert.AnError,
			},
			expectedError: "failed to apply TTL to trace_id_timestamps",
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
				DialTimeout: 1 * time.Second,
				Schema: Schema{
					Create:   false,
					TraceTTL: tt.traceTTL,
				},
			}

			f, err := NewFactory(context.Background(), cfg, telemetry.Settings{})
			require.ErrorContains(t, err, tt.expectedError)
			require.Nil(t, f)
		})
	}
}

func TestExecuteTemplate(t *testing.T) {
	tests := []struct {
		name        string
		templateSQL string
		data        map[string]any
		wantErr     bool
		errContains string
	}{
		{
			name:        "valid template without TTL",
			templateSQL: "CREATE TABLE test{{- if .TTLSeconds }} TTL {{ .TTLSeconds }}{{- end }}",
			data:        map[string]any{"TTLSeconds": int64(0)},
			wantErr:     false,
		},
		{
			name:        "valid template with TTL",
			templateSQL: "CREATE TABLE test{{- if .TTLSeconds }} TTL {{ .TTLSeconds }}{{- end }}",
			data:        map[string]any{"TTLSeconds": int64(86400)},
			wantErr:     false,
		},
		{
			name:        "invalid template syntax - parse error",
			templateSQL: "CREATE TABLE test {{- if .Invalid }",
			data:        map[string]any{"TTLSeconds": int64(0)},
			wantErr:     true,
			errContains: "failed to parse",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := executeTemplate("test", tt.templateSQL, tt.data)
			if tt.wantErr {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.errContains)
			} else {
				require.NoError(t, err)
				require.NotEmpty(t, result)
			}
		})
	}
}

func TestApplyTTL_ZeroTTL(t *testing.T) {
	srv := clickhousetest.NewServer(clickhousetest.FailureConfig{})
	defer srv.Close()

	cfg := Configuration{
		Protocol: "http",
		Addresses: []string{
			srv.Listener.Addr().String(),
		},
		Database: "default",
		Schema: Schema{
			Create:   false,
			TraceTTL: 0, // Zero TTL - should not apply TTL
		},
	}

	f, err := NewFactory(context.Background(), cfg, telemetry.Settings{})
	require.NoError(t, err)
	require.NotNil(t, f)
	require.NoError(t, f.Close())
}

func TestNewFactory_TemplateErrors(t *testing.T) {
	// Save original values and restore them after the test
	originalSpansTable := sql.CreateSpansTable
	originalTraceIDTimestampsTable := sql.CreateTraceIDTimestampsTable
	defer func() {
		sql.CreateSpansTable = originalSpansTable
		sql.CreateTraceIDTimestampsTable = originalTraceIDTimestampsTable
	}()

	tests := []struct {
		name          string
		setup         func()
		expectedError string
	}{
		{
			name: "spans table template error",
			setup: func() {
				sql.CreateSpansTable = "INVALID TEMPLATE {{ if }"
			},
			expectedError: "failed to parse spans_table template",
		},
		{
			name: "trace id timestamps table template error",
			setup: func() {
				sql.CreateTraceIDTimestampsTable = "INVALID TEMPLATE {{ if }"
			},
			expectedError: "failed to parse trace_id_timestamps_table template",
		},
		{
			name: "spans table template execution error",
			setup: func() {
				// {{ call .TTLSeconds }} will fail because TTLSeconds is not a function
				sql.CreateSpansTable = "{{ call .TTLSeconds }}"
			},
			expectedError: "failed to execute spans_table template",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset to originals before each subtest to ensure clean state
			sql.CreateSpansTable = originalSpansTable
			sql.CreateTraceIDTimestampsTable = originalTraceIDTimestampsTable

			tt.setup()

			srv := clickhousetest.NewServer(clickhousetest.FailureConfig{})
			defer srv.Close()

			cfg := Configuration{
				Protocol: "http",
				Addresses: []string{
					srv.Listener.Addr().String(),
				},
				Database: "default",
				Schema: Schema{
					Create: true,
				},
				DialTimeout: 1 * time.Second,
			}

			f, err := NewFactory(context.Background(), cfg, telemetry.Settings{})
			require.Error(t, err)
			require.Contains(t, err.Error(), tt.expectedError)
			require.Nil(t, f)
		})
	}
}
