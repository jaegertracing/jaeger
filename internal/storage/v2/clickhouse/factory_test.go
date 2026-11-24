// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package clickhouse

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"slices"
	"strings"
	"sync/atomic"
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
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"github.com/jaegertracing/jaeger/internal/metrics"
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

// buildSchema renders the schema for cfg as NewFactory does, with the attribute layout and the
// hoisted columns that cfg selects.
func buildSchema(cfg Configuration) (*schemaBuilder, error) {
	layout, err := cfg.attributeLayout()
	if err != nil {
		return nil, err
	}
	hoistedColumns, err := newHoistedColumns(cfg.HoistedAttributes, layout)
	if err != nil {
		return nil, err
	}
	return newSchemaBuilder(cfg, layout, hoistedColumns)
}

// schemaQuery returns the statement newSchemaBuilder renders under the given name for
// localTableEngine and no TTL, so the tests expect exactly what the factory executes.
func schemaQuery(t *testing.T, name string) string {
	builder, err := buildSchema(Configuration{CreateSchema: true, TableEngine: localTableEngine})
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
				schemaQuery(t, "services materialized view"): assert.AnError,
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
				schemaQuery(t, "operations materialized view"): assert.AnError,
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
				schemaQuery(t, "trace id timestamps materialized view"): assert.AnError,
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
		cfg           Configuration
		mockFn        func(name, tmplBody string, data any) (string, error)
		expectedError string
	}{
		{
			name: "first loadTemplate call fails",
			cfg:  Configuration{CreateSchema: true, TableEngine: localTableEngine},
			mockFn: func(_, _ string, _ any) (string, error) {
				return "", errors.New("mock template error")
			},
			expectedError: "mock template error",
		},
		{
			name: "second loadTemplate call fails",
			cfg:  Configuration{CreateSchema: true, TableEngine: localTableEngine},
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
		{
			// The second call under a distributed layout renders the first table's
			// Distributed table, a different template than the second call renders
			// without one.
			name: "distributed table loadTemplate call fails",
			cfg: Configuration{
				CreateSchema: true,
				Database:     "jaeger",
				TableEngine: TableEngine{
					Replicated: configoptional.Some(ReplicatedEngine{}),
					Cluster:    configoptional.Some(ClusterConfig{Name: "my_cluster"}),
				},
			},
			mockFn: func() func(name, tmplBody string, data any) (string, error) {
				calls := 0
				return func(name, tmplBody string, data any) (string, error) {
					calls++
					if calls == 2 {
						return "", errors.New("mock distributed template error")
					}
					return loadTemplateImpl(name, tmplBody, data)
				}
			}(),
			expectedError: "mock distributed template error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loadTemplate = tt.mockFn
			_, err := NewFactory(context.Background(), tt.cfg, telemetry.NoopSettings())
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
		queryWithTTL, err := loadTemplate("test_ttl", sql.CreateSpansTable, schemaTemplateParams{TTLSeconds: 86400, MergeTree: "MergeTree", layout: chtracestore.NestedLayout{}})
		require.NoError(t, err)
		assert.Contains(t, queryWithTTL, "TTL start_time + INTERVAL 86400 SECOND DELETE")
	})
}

// TestSchemaBuilder_AttributeLayout snapshots, for each attribute layout, the statements
// that depend on it: the spans table and the three views that collect attribute keys. The
// spans table has one hoisted column per stored type and level, a key that needs escaping,
// a column name that is a keyword, and a codec that ClickHouse stores in another spelling.
func TestSchemaBuilder_AttributeLayout(t *testing.T) {
	for _, layout := range []string{"nested", "map"} {
		t.Run(layout+" layout", func(t *testing.T) {
			builder, err := buildSchema(Configuration{
				CreateSchema:    true,
				TableEngine:     localTableEngine,
				AttributeLayout: layout,
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
		})
	}
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

// hoistedColumnServer is a test server that has a spans table and no spans_local table. Its
// spans table declares every hoisted column as state, and the server records the queries it
// receives.
func hoistedColumnServer(t *testing.T, state hoistedColumnState) (srv *httptest.Server, queries *[]string) {
	fakeServer := clickhousetest.NewServer(clickhousetest.FailureConfig{})
	t.Cleanup(fakeServer.Close)
	queries = new([]string)
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		*queries = append(*queries, string(body))
		block := chproto.NewBlock()
		switch {
		case strings.Contains(string(body), "same_expression"):
			block.AddColumn("type", "String")
			block.AddColumn("default_kind", "String")
			block.AddColumn("default_expression", "String")
			block.AddColumn("compression_codec", "String")
			block.AddColumn("same_expression", "UInt8")
			block.Append(state.Type, state.Kind, state.Expression, state.Codec, state.SameExpression)
		case strings.Contains(string(body), "FROM system.tables"):
			block.AddColumn("name", "String")
			block.AddColumn("engine", "String")
			block.Append("spans", "MergeTree")
		default:
			r.Body = io.NopCloser(bytes.NewReader(body))
			fakeServer.Config.Handler.ServeHTTP(w, r)
			return
		}
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

func TestNewFactory_InvalidAttributeLayout(t *testing.T) {
	_, err := NewFactory(context.Background(), Configuration{AttributeLayout: "json"}, telemetry.NoopSettings())
	require.ErrorContains(t, err, "attribute_layout must be nested or map")
}

// TestNewFactory_InvalidCluster checks that NewFactory rejects an invalid cluster block
// before attempting to connect, the same way it rejects an invalid attribute layout.
func TestNewFactory_InvalidCluster(t *testing.T) {
	cfg := Configuration{
		CreateSchema: true,
		TableEngine: TableEngine{
			MergeTree: configoptional.Some(MergeTreeEngine{}),
			Cluster:   configoptional.Some(ClusterConfig{Name: "it's"}),
		},
	}
	_, err := NewFactory(context.Background(), cfg, telemetry.NoopSettings())
	require.ErrorContains(t, err, "`table_engine.cluster.name` must not contain a single quote or a backslash")
}

// TestNewFactory_MergeTreeClusterWarns checks that NewFactory logs a warning, rather than
// failing, when the cluster block distributes a merge_tree engine, which shards without
// replicas.
func TestNewFactory_MergeTreeClusterWarns(t *testing.T) {
	srv := clickhousetest.NewServer(clickhousetest.FailureConfig{})
	defer srv.Close()

	core, observed := observer.New(zap.WarnLevel)
	cfg := DefaultConfiguration()
	cfg.Protocol = "http"
	cfg.Database = "jaeger"
	cfg.Addresses = []string{srv.Listener.Addr().String()}
	cfg.CreateSchema = true
	cfg.TableEngine = TableEngine{
		MergeTree: configoptional.Some(MergeTreeEngine{}),
		Cluster:   configoptional.Some(ClusterConfig{Name: "my_cluster"}),
	}
	f, err := NewFactory(context.Background(), cfg, telemetry.Settings{
		Logger:  zap.New(core),
		Metrics: metrics.NullFactory,
	})
	require.NoError(t, err)
	require.NoError(t, f.Close())

	require.Equal(t, 1, observed.Len())
	assert.Contains(t, observed.All()[0].Message, "the data has no second copy")
}

// TestNewFactory_AttributeLayoutMismatch checks that the factory runs the layout check
// before it creates the schema: the test server reports a spans table in the map layout, and
// the configuration is nested. A view of the other layout would fail to create with an error
// that does not name the layout.
func TestNewFactory_AttributeLayoutMismatch(t *testing.T) {
	fakeServer := clickhousetest.NewServer(clickhousetest.FailureConfig{})
	t.Cleanup(fakeServer.Close)
	var schemaStatements atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if strings.Contains(string(body), "CREATE ") {
			schemaStatements.Add(1)
		}
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
	cfg.CreateSchema = true
	cfg.TableEngine = localTableEngine

	_, err := NewFactory(context.Background(), cfg, telemetry.NoopSettings())

	require.ErrorContains(t, err, "the spans table declares its string attributes as Map(String, String), which is not the nested layout")
	assert.Zero(t, schemaStatements.Load())
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
			builder, err := buildSchema(Configuration{CreateSchema: true, TableEngine: tt.engine})
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
		_, err := buildSchema(Configuration{CreateSchema: true})
		require.ErrorContains(t, err, "`create_schema` requires `table_engine`")
	})
}

// TestSchemaBuilder_Distributed snapshots the whole schema with and without
// table_engine.cluster, one file per case with each statement under its name, so that the
// two layouts can be compared side by side.
func TestSchemaBuilder_Distributed(t *testing.T) {
	tests := []struct {
		name        string
		tableEngine TableEngine
	}{
		{
			name: "with cluster",
			tableEngine: TableEngine{
				Replicated: configoptional.Some(ReplicatedEngine{}),
				Cluster:    configoptional.Some(ClusterConfig{Name: "my_cluster"}),
			},
		},
		{
			name:        "without cluster",
			tableEngine: localTableEngine,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder, err := buildSchema(Configuration{
				CreateSchema: true,
				TableEngine:  tt.tableEngine,
			})
			require.NoError(t, err)
			var schema strings.Builder
			for _, stmt := range builder.statements {
				fmt.Fprintf(&schema, "-- %s\n%s\n\n", stmt.name, strings.TrimSpace(stmt.query))
			}
			clickhousetest.VerifyQuerySnapshot(t, schema.String())
		})
	}
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

// TestSchemaTemplateFunctions_UnknownValue checks that a template which names a level or a
// type of attribute group that does not exist fails to render.
func TestSchemaTemplateFunctions_UnknownValue(t *testing.T) {
	params := schemaTemplateParams{layout: chtracestore.MapLayout{}}
	for template, wantErr := range map[string]string{
		`{{ .ColumnTypeFn "bytes" }}`: `"bytes" is not a type of attribute group`,
		`{{ .KeysFn "span" "text" }}`: `"text" is not a type of attribute group`,
		`{{ .KeysFn "trace" "str" }}`: `"trace" is not a level of attributes`,
	} {
		_, err := loadTemplate("test", template, params)
		require.ErrorContains(t, err, wantErr)
	}
}
