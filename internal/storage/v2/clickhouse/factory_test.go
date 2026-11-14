// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package clickhouse

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ClickHouse/ch-go/proto"
	"github.com/ClickHouse/clickhouse-go/v2"
	chproto "github.com/ClickHouse/clickhouse-go/v2/lib/proto"
	"github.com/open-telemetry/opentelemetry-collector-contrib/extension/basicauthextension"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/config/configoptional"
	"go.opentelemetry.io/collector/config/configtls"
	"go.opentelemetry.io/collector/pdata/pcommon"

	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/clickhousetest"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/sql"
	chtracestore "github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/tracestore"
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
				CreateSchema: tt.createSchema,
				TableEngine:  localTableEngine,
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

// localTableEngine is the engine the unit tests create the schema with.
var localTableEngine = TableEngine{
	MergeTree: configoptional.Some(MergeTreeEngine{}),
}

// schemaQuery returns the statement newSchemaBuilder renders under the given name for
// localTableEngine and no TTL, so the tests expect exactly what the factory executes.
func schemaQuery(t *testing.T, name string) string {
	builder, err := newSchemaBuilder(Configuration{CreateSchema: true, TableEngine: localTableEngine})
	require.NoError(t, err)
	for _, stmt := range builder.statements {
		if stmt.name == name {
			return stmt.query
		}
	}
	require.Failf(t, "unknown schema statement", "%s", name)
	return ""
}

func TestNewFactory_Errors(t *testing.T) {
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
				schemaQuery(t, "spans table"): assert.AnError,
			},
			expectedError: "failed to create spans table",
		},
		{
			name: "services table creation error",
			failureConfig: clickhousetest.FailureConfig{
				schemaQuery(t, "services table"): assert.AnError,
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
				schemaQuery(t, "operations table"): assert.AnError,
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
				schemaQuery(t, "trace id timestamps table"): assert.AnError,
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
				schemaQuery(t, "attribute metadata table"): assert.AnError,
			},
			expectedError: "failed to create attribute metadata table",
		},
		{
			name: "attribute metadata materialized view creation error",
			failureConfig: clickhousetest.FailureConfig{
				schemaQuery(t, "attribute metadata materialized view"): assert.AnError,
			},
			expectedError: "failed to create attribute metadata materialized view",
		},
		{
			name: "event attribute metadata materialized view creation error",
			failureConfig: clickhousetest.FailureConfig{
				schemaQuery(t, "event attribute metadata materialized view"): assert.AnError,
			},
			expectedError: "failed to create event attribute metadata materialized view",
		},
		{
			name: "link attribute metadata materialized view creation error",
			failureConfig: clickhousetest.FailureConfig{
				schemaQuery(t, "link attribute metadata materialized view"): assert.AnError,
			},
			expectedError: "failed to create link attribute metadata materialized view",
		},
		{
			name: "dependencies table creation error",
			failureConfig: clickhousetest.FailureConfig{
				schemaQuery(t, "dependencies table"): assert.AnError,
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
				TableEngine:  localTableEngine,
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
				TableEngine:  localTableEngine,
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
				Configuration{CreateSchema: true, TableEngine: localTableEngine},
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
		queryWithoutTTL := schemaQuery(t, "spans table")
		assert.NotContains(t, queryWithoutTTL, "TTL start_time")
	})

	t.Run("with TTL", func(t *testing.T) {
		queryWithTTL, err := loadTemplate("test_ttl", sql.CreateSpansTable, schemaTemplateParams{TTLSeconds: 86400, MergeTree: "MergeTree", Layout: chtracestore.NestedLayout{}})
		require.NoError(t, err)
		assert.Contains(t, queryWithTTL, "TTL start_time + INTERVAL 86400 SECOND DELETE")
	})
}

// TestCreateSpansTable_HoistedColumns snapshots the spans table with one hoisted column per
// stored type and level, a key that needs escaping, a column name that is a keyword, and a
// codec that ClickHouse stores in another spelling.
func TestCreateSpansTable_HoistedColumns(t *testing.T) {
	builder, err := newSchemaBuilder(Configuration{
		CreateSchema: true,
		TableEngine:  localTableEngine,
		HoistedAttributes: []HoistedAttribute{
			{
				Key:    "gen_ai.request.model",
				Level:  "span",
				Type:   "string",
				Column: "span_str",
			},
			{
				Key:    "gen_ai.usage.input_tokens",
				Level:  "span",
				Type:   "int",
				Column: "span_int",
			},
			{
				Key:    "ratio",
				Level:  "span",
				Type:   "double",
				Column: "span_double",
			},
			{
				Key:    "cached",
				Level:  "span",
				Type:   "bool",
				Column: "span_bool",
			},
			{
				Key:    "gen_ai.request.model",
				Level:  "resource",
				Type:   "string",
				Column: "resource_str",
			},
			{
				Key:    "shard",
				Level:  "resource",
				Type:   "int",
				Column: "resource_int",
				Codec:  "ZSTD(03)",
			},
			{
				Key:    `it's a \key`,
				Level:  "resource",
				Type:   "string",
				Column: "index",
			},
		},
	})
	require.NoError(t, err)
	clickhousetest.VerifyQuerySnapshot(t, builder.statements[0].query)
}

func TestNewSchemaBuilder_InvalidHoistedAttribute(t *testing.T) {
	_, err := newSchemaBuilder(Configuration{
		CreateSchema:      true,
		TableEngine:       localTableEngine,
		HoistedAttributes: []HoistedAttribute{{Key: "k"}},
	})
	require.ErrorContains(t, err, "hoisted_attributes[0]")
}

// TestNewFactory_SpansTableMissing checks that the factory runs the check: the test server
// answers every query with no rows, so neither the configured column nor the table is found.
func TestNewFactory_SpansTableMissing(t *testing.T) {
	srv := clickhousetest.NewServer(clickhousetest.FailureConfig{})
	defer srv.Close()

	_, err := NewFactory(context.Background(), Configuration{
		Protocol:  "http",
		Addresses: []string{srv.Listener.Addr().String()},
		Database:  "default",
		HoistedAttributes: []HoistedAttribute{
			{
				Key:    "k",
				Level:  "span",
				Type:   "string",
				Column: "k_column",
			},
		},
	}, telemetry.NoopSettings())
	require.ErrorContains(t, err, "the spans table does not exist")
}

// hoistedColumnServer is a test server whose spans table declares every hoisted column as
// state, and which records the queries it receives.
func hoistedColumnServer(t *testing.T, state hoistedColumnState) (srv *httptest.Server, queries *[]string) {
	fakeServer := clickhousetest.NewServer(clickhousetest.FailureConfig{})
	t.Cleanup(fakeServer.Close)
	queries = new([]string)
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		*queries = append(*queries, string(body))
		if !strings.Contains(string(body), "same_expression") {
			r.Body = io.NopCloser(bytes.NewReader(body))
			fakeServer.Config.Handler.ServeHTTP(w, r)
			return
		}
		block := chproto.NewBlock()
		block.AddColumn("type", "String")
		block.AddColumn("default_kind", "String")
		block.AddColumn("default_expression", "String")
		block.AddColumn("compression_codec", "String")
		block.AddColumn("same_expression", "UInt8")
		block.Append(state.Type, state.Kind, state.Expression, state.Codec, state.SameExpression)
		var buf proto.Buffer
		block.Encode(&buf, clickhouse.ClientTCPProtocolVersion)
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(buf.Buf)
	}))
	t.Cleanup(srv.Close)
	return srv, queries
}

// TestNewFactory_HoistedColumns checks that the factory creates the schema before it checks the
// columns, and starts when the spans table declares them as configured.
func TestNewFactory_HoistedColumns(t *testing.T) {
	srv, queries := hoistedColumnServer(t, hoistedColumnState{
		Type:           "Nullable(Int64)",
		Kind:           "MATERIALIZED",
		Codec:          "CODEC(ZSTD(1))",
		SameExpression: 1,
	})
	cfg := DefaultConfiguration()
	cfg.Protocol = "http"
	cfg.Addresses = []string{srv.Listener.Addr().String()}
	cfg.CreateSchema = true
	cfg.TableEngine = localTableEngine
	cfg.HoistedAttributes = []HoistedAttribute{
		{
			Key:    "gen_ai.usage.input_tokens",
			Level:  "span",
			Type:   "int",
			Column: "input_tokens",
		},
	}

	factory, err := NewFactory(context.Background(), cfg, telemetry.NoopSettings())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, factory.Close()) })

	createQueryIndex := slices.IndexFunc(*queries, func(q string) bool {
		return strings.Contains(q, "IF NOT EXISTS spans (")
	})
	checkQueryIndex := slices.IndexFunc(*queries, func(q string) bool {
		return strings.Contains(q, "same_expression")
	})
	require.NotEqual(t, -1, createQueryIndex)
	require.Greater(t, checkQueryIndex, createQueryIndex)
	// The driver writes the bind values into the query, so the text shows which value went to
	// which placeholder.
	assert.Contains(t, (*queries)[checkQueryIndex], "name = 'input_tokens'")
	assert.Contains(t, (*queries)[checkQueryIndex], `concat('SELECT ', 'if(has(int_attributes.key, \'gen_ai.usage.input_tokens\')`)
}

func TestNewFactory_InvalidHoistedAttribute(t *testing.T) {
	_, err := NewFactory(context.Background(), Configuration{
		HoistedAttributes: []HoistedAttribute{{Key: "k"}},
	}, telemetry.NoopSettings())
	require.ErrorContains(t, err, "hoisted_attributes[0]")
}

// TestCreateTraceReader_HoistedColumns checks that a reader created by the factory searches
// the configured hoisted columns.
func TestCreateTraceReader_HoistedColumns(t *testing.T) {
	srv, queries := hoistedColumnServer(t, hoistedColumnState{
		Type:           "Nullable(Int64)",
		Kind:           "MATERIALIZED",
		Codec:          "CODEC(ZSTD(1))",
		SameExpression: 1,
	})
	cfg := DefaultConfiguration()
	cfg.Protocol = "http"
	cfg.Addresses = []string{srv.Listener.Addr().String()}
	cfg.HoistedAttributes = []HoistedAttribute{
		{
			Key:    "gen_ai.usage.input_tokens",
			Level:  "span",
			Type:   "int",
			Column: "input_tokens",
		},
	}
	factory, err := NewFactory(context.Background(), cfg, telemetry.NoopSettings())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, factory.Close()) })
	reader, err := factory.CreateTraceReader()
	require.NoError(t, err)

	attributes := pcommon.NewMap()
	attributes.PutInt("gen_ai.usage.input_tokens", 7)
	for _, err := range reader.FindTraceIDs(context.Background(), tracestore.TraceQueryParams{Attributes: attributes}) {
		require.NoError(t, err)
	}

	// The driver writes the bind value into the query.
	assert.Contains(t, (*queries)[len(*queries)-1], "(s.`input_tokens` IS NOT NULL AND s.`input_tokens` = 7)")
}

// TestSchemaBuilder_MapLayout snapshots the statements that depend on the attribute layout,
// rendered for the map layout: the spans table with a hoisted column of each level, and the
// three views that collect attribute keys.
func TestSchemaBuilder_MapLayout(t *testing.T) {
	builder, err := newSchemaBuilder(Configuration{
		CreateSchema:    true,
		TableEngine:     localTableEngine,
		AttributeLayout: "map",
		HoistedAttributes: []HoistedAttribute{
			{
				Key:    "gen_ai.request.model",
				Level:  "span",
				Type:   "string",
				Column: "span_str",
			},
			{
				Key:    `it's a \key`,
				Level:  "resource",
				Type:   "int",
				Column: "resource_int",
			},
		},
	})
	require.NoError(t, err)
	var queries []string
	for _, statement := range builder.statements {
		switch statement.name {
		case "spans table",
			"attribute metadata materialized view",
			"event attribute metadata materialized view",
			"link attribute metadata materialized view":
			queries = append(queries, statement.query)
		default:
		}
	}
	clickhousetest.VerifyQuerySnapshot(t, queries...)
}

func TestNewFactory_InvalidAttributeLayout(t *testing.T) {
	_, err := NewFactory(context.Background(), Configuration{AttributeLayout: "json"}, telemetry.NoopSettings())
	require.ErrorContains(t, err, "attribute_layout must be nested or map")
}

func TestNewSchemaBuilder_InvalidAttributeLayout(t *testing.T) {
	_, err := newSchemaBuilder(Configuration{
		CreateSchema:    true,
		TableEngine:     localTableEngine,
		AttributeLayout: "json",
	})
	require.ErrorContains(t, err, "attribute_layout must be nested or map")
}

// TestNewFactory_AttributeLayoutMismatch checks that the factory runs the layout check: the
// test server reports a spans table in the map layout, and the configuration is nested.
func TestNewFactory_AttributeLayoutMismatch(t *testing.T) {
	fakeServer := clickhousetest.NewServer(clickhousetest.FailureConfig{})
	t.Cleanup(fakeServer.Close)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), "name IN ('str_attributes'") {
			r.Body = io.NopCloser(bytes.NewReader(body))
			fakeServer.Config.Handler.ServeHTTP(w, r)
			return
		}
		block := chproto.NewBlock()
		block.AddColumn("type", "String")
		block.Append("Map(String, String)")
		var buf proto.Buffer
		block.Encode(&buf, clickhouse.ClientTCPProtocolVersion)
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(buf.Buf)
	}))
	t.Cleanup(srv.Close)
	cfg := DefaultConfiguration()
	cfg.Protocol = "http"
	cfg.Addresses = []string{srv.Listener.Addr().String()}

	_, err := NewFactory(context.Background(), cfg, telemetry.NoopSettings())

	require.ErrorContains(t, err, "the spans table declares its string attributes as Map(String, String), which is not the nested layout")
}

func TestCreateTraceIDTimestampsTableTemplate(t *testing.T) {
	t.Run("without TTL", func(t *testing.T) {
		queryWithoutTTL := schemaQuery(t, "trace id timestamps table")
		assert.NotContains(t, queryWithoutTTL, "TTL end")
	})

	t.Run("with TTL", func(t *testing.T) {
		queryWithTTL, err := loadTemplate("test_ttl_trace", sql.CreateTraceIDTimestampsTable, schemaTemplateParams{TTLSeconds: 86400, AggregatingMergeTree: "AggregatingMergeTree"})
		require.NoError(t, err)
		assert.Contains(t, queryWithTTL, "TTL end + INTERVAL 86400 SECOND DELETE")
	})
}

// TestSchemaBuilder_Engines checks that every table the builder creates carries the ENGINE
// clause the configured TableEngine renders, and that the materialized views carry none.
func TestSchemaBuilder_Engines(t *testing.T) {
	mergeTreeTables := map[string]bool{
		"spans table":        true,
		"dependencies table": true,
	}
	aggregatingTables := map[string]bool{
		"services table":            true,
		"operations table":          true,
		"trace id timestamps table": true,
		"attribute metadata table":  true,
	}
	tests := []struct {
		name            string
		engine          TableEngine
		wantMergeTree   string
		wantAggregating string
	}{
		{
			name:            "merge_tree",
			engine:          localTableEngine,
			wantMergeTree:   "ENGINE = MergeTree",
			wantAggregating: "ENGINE = AggregatingMergeTree",
		},
		{
			name: "replicated with server defaults",
			engine: TableEngine{
				Replicated: configoptional.Some(ReplicatedEngine{}),
			},
			wantMergeTree:   "ENGINE = ReplicatedMergeTree",
			wantAggregating: "ENGINE = ReplicatedAggregatingMergeTree",
		},
		{
			name: "replicated with keeper path and replica name",
			engine: TableEngine{
				Replicated: configoptional.Some(ReplicatedEngine{
					KeeperPath:  "/clickhouse/tables/{shard}/{database}/{table}",
					ReplicaName: "{replica}",
				}),
			},
			wantMergeTree:   "ENGINE = ReplicatedMergeTree('/clickhouse/tables/{shard}/{database}/{table}', '{replica}')",
			wantAggregating: "ENGINE = ReplicatedAggregatingMergeTree('/clickhouse/tables/{shard}/{database}/{table}', '{replica}')",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder, err := newSchemaBuilder(Configuration{CreateSchema: true, TableEngine: tt.engine})
			require.NoError(t, err)
			seen := 0
			for _, stmt := range builder.statements {
				assert.NotContains(t, stmt.query, "{{", stmt.name)
				// The clause must end at whitespace, so that a bare ReplicatedMergeTree
				// is not satisfied by ReplicatedMergeTree('...') and vice versa.
				switch {
				case mergeTreeTables[stmt.name]:
					assert.Regexp(t, regexp.QuoteMeta(tt.wantMergeTree)+`\s`, stmt.query, stmt.name)
					seen++
				case aggregatingTables[stmt.name]:
					assert.Regexp(t, regexp.QuoteMeta(tt.wantAggregating)+`\s`, stmt.query, stmt.name)
					seen++
				default:
					assert.NotContains(t, stmt.query, "ENGINE", stmt.name)
				}
			}
			assert.Equal(t, len(mergeTreeTables)+len(aggregatingTables), seen)
		})
	}

	t.Run("unset engine is refused", func(t *testing.T) {
		_, err := newSchemaBuilder(Configuration{CreateSchema: true})
		require.ErrorContains(t, err, "`create_schema` requires `table_engine`")
	})
}

func TestNewFactory_KeepsExplicitZeroCacheSettings(t *testing.T) {
	srv := clickhousetest.NewServer(clickhousetest.FailureConfig{})
	defer srv.Close()

	cfg := DefaultConfiguration()
	cfg.Protocol = "http"
	cfg.Addresses = []string{srv.Listener.Addr().String()}
	cfg.AttributeMetadataCacheTTL = 0
	cfg.AttributeMetadataCacheMaxSize = 0

	f, err := NewFactory(context.Background(), cfg, telemetry.NoopSettings())
	require.NoError(t, err)
	defer func() { require.NoError(t, f.Close()) }()

	assert.Zero(t, f.config.AttributeMetadataCacheTTL)
	assert.Zero(t, f.config.AttributeMetadataCacheMaxSize)
}
