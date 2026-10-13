// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package clickhouse

import (
	"bytes"
	"context"
	dbsql "database/sql"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ClickHouse/ch-go/proto"
	"github.com/ClickHouse/clickhouse-go/v2"
	chproto "github.com/ClickHouse/clickhouse-go/v2/lib/proto"
	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database"
	clickhousemigrate "github.com/golang-migrate/migrate/v4/database/clickhouse"
	"github.com/golang-migrate/migrate/v4/source"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/config/configoptional"
	"go.opentelemetry.io/collector/config/configtls"
	"go.opentelemetry.io/collector/pdata/pcommon"

	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/clickhousetest"
	chsql "github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/sql"
	"github.com/jaegertracing/jaeger/internal/telemetry"
)

var localTableEngine = TableEngine{
	MergeTree: configoptional.Some(MergeTreeEngine{}),
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

	loadTemplate = func(_, _ string, _ any) (string, error) {
		return "", errors.New("mock template error")
	}

	opts := &clickhouse.Options{}
	_, err := newSchemaBuilder(
		Configuration{CreateSchema: true, TableEngine: localTableEngine},
		opts,
	)
	require.ErrorContains(t, err, "mock template error")
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
		queryWithoutTTL, err := loadTemplate("test_no_ttl", chsql.CreateSpansTable, schemaTemplateParams{TTLSeconds: 0, MergeTree: "MergeTree"})
		require.NoError(t, err)
		assert.NotContains(t, queryWithoutTTL, "TTL start_time")
	})

	t.Run("with TTL", func(t *testing.T) {
		queryWithTTL, err := loadTemplate("test_ttl", chsql.CreateSpansTable, schemaTemplateParams{TTLSeconds: 86400, MergeTree: "MergeTree"})
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
	}, nil)
	require.NoError(t, err)
	sqlBytes, err := fs.ReadFile(builder.renderedFS, "000001_initial_schema.up.sql")
	require.NoError(t, err)
	spansQuery := strings.Split(string(sqlBytes), ";")[0]
	clickhousetest.VerifyQuerySnapshot(t, spansQuery)
}

func TestNewSchemaBuilder_InvalidHoistedAttribute(t *testing.T) {
	_, err := newSchemaBuilder(Configuration{
		CreateSchema:      true,
		TableEngine:       localTableEngine,
		HoistedAttributes: []HoistedAttribute{{Key: "k"}},
	}, nil)
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
		if !strings.Contains(string(body), "FROM system.columns") {
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
		return strings.Contains(q, "FROM system.columns")
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

func TestCreateTraceIDTimestampsTableTemplate(t *testing.T) {
	t.Run("without TTL", func(t *testing.T) {
		queryWithoutTTL, err := loadTemplate("test_no_ttl_trace", chsql.CreateTraceIDTimestampsTable, schemaTemplateParams{TTLSeconds: 0, AggregatingMergeTree: "AggregatingMergeTree"})
		require.NoError(t, err)
		assert.NotContains(t, queryWithoutTTL, "TTL end")
	})

	t.Run("with TTL", func(t *testing.T) {
		queryWithTTL, err := loadTemplate("test_ttl_trace", chsql.CreateTraceIDTimestampsTable, schemaTemplateParams{TTLSeconds: 86400, AggregatingMergeTree: "AggregatingMergeTree"})
		require.NoError(t, err)
		assert.Contains(t, queryWithTTL, "TTL end + INTERVAL 86400 SECOND DELETE")
	})
}

// TestSchemaBuilder_Engines checks that every table in the rendered migration carries the ENGINE
// clause the configured TableEngine renders, and that the materialized views carry none.
func TestSchemaBuilder_Engines(t *testing.T) {
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
			builder, err := newSchemaBuilder(Configuration{CreateSchema: true, TableEngine: tt.engine}, nil)
			require.NoError(t, err)

			sqlBytes, err := fs.ReadFile(builder.renderedFS, "000001_initial_schema.up.sql")
			require.NoError(t, err)
			sqlContent := string(sqlBytes)

			assert.NotContains(t, sqlContent, "{{")
			assert.Contains(t, sqlContent, tt.wantMergeTree)
			assert.Contains(t, sqlContent, tt.wantAggregating)
		})
	}

	t.Run("unset engine is refused", func(t *testing.T) {
		_, err := newSchemaBuilder(Configuration{CreateSchema: true}, nil)
		require.ErrorContains(t, err, "`create_schema` requires `table_engine`")
	})
}

func TestSchemaBuilder_TTL(t *testing.T) {
	t.Run("renders TTL when configured", func(t *testing.T) {
		cfg := Configuration{
			CreateSchema: true,
			TableEngine:  localTableEngine,
			TTL:          24 * time.Hour,
		}
		builder, err := newSchemaBuilder(cfg, nil)
		require.NoError(t, err)

		sqlBytes, err := fs.ReadFile(builder.renderedFS, "000001_initial_schema.up.sql")
		require.NoError(t, err)
		sqlContent := string(sqlBytes)

		assert.Contains(t, sqlContent, "TTL start_time + INTERVAL 86400 SECOND DELETE")
		assert.Contains(t, sqlContent, "TTL end + INTERVAL 86400 SECOND DELETE")
	})

	t.Run("omits TTL when zero", func(t *testing.T) {
		cfg := Configuration{
			CreateSchema: true,
			TableEngine:  localTableEngine,
			TTL:          0,
		}
		builder, err := newSchemaBuilder(cfg, nil)
		require.NoError(t, err)

		sqlBytes, err := fs.ReadFile(builder.renderedFS, "000001_initial_schema.up.sql")
		require.NoError(t, err)
		sqlContent := string(sqlBytes)

		assert.NotContains(t, sqlContent, "TTL start_time")
		assert.NotContains(t, sqlContent, "TTL end")
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

func TestIsTableNotExist(t *testing.T) {
	assert.False(t, isTableNotExist(nil))
	assert.False(t, isTableNotExist(errors.New("other error")))
	assert.False(t, isTableNotExist(&clickhouse.Exception{Code: 50}))
	assert.True(t, isTableNotExist(&clickhouse.Exception{Code: 60}))
}

func TestReadSchemaVersionImpl(t *testing.T) {
	t.Run("table does not exist returns version 0", func(t *testing.T) {
		srv := clickhousetest.NewServer(clickhousetest.FailureConfig{
			selectSchemaVersionQuery: errors.New("Code: 60. DB::Exception: Table default.schema_migrations doesn't exist. (UNKNOWN_TABLE)"),
		})
		defer srv.Close()
		cfg := Configuration{
			Protocol:  "http",
			Addresses: []string{srv.Listener.Addr().String()},
		}
		opts, err := Options(context.Background(), cfg)
		require.NoError(t, err)
		db := clickhouse.OpenDB(opts)
		defer db.Close()

		v, dirty, err := readSchemaVersionImpl(context.Background(), db)
		require.NoError(t, err)
		assert.Equal(t, uint(0), v)
		assert.False(t, dirty)
	})

	t.Run("valid row returns schema version", func(t *testing.T) {
		srv := clickhousetest.NewServer(clickhousetest.FailureConfig{})
		defer srv.Close()
		cfg := Configuration{
			Protocol:  "http",
			Addresses: []string{srv.Listener.Addr().String()},
		}
		opts, err := Options(context.Background(), cfg)
		require.NoError(t, err)
		db := clickhouse.OpenDB(opts)
		defer db.Close()

		v, dirty, err := readSchemaVersionImpl(context.Background(), db)
		require.NoError(t, err)
		assert.Equal(t, uint(1), v)
		assert.False(t, dirty)
	})

	t.Run("no rows returns version 0", func(t *testing.T) {
		orig := selectSchemaVersionQuery
		defer func() { selectSchemaVersionQuery = orig }()
		selectSchemaVersionQuery = "SELECT version, dirty FROM empty_migrations"

		srv := clickhousetest.NewServer(clickhousetest.FailureConfig{})
		defer srv.Close()
		cfg := Configuration{
			Protocol:  "http",
			Addresses: []string{srv.Listener.Addr().String()},
		}
		opts, err := Options(context.Background(), cfg)
		require.NoError(t, err)
		db := clickhouse.OpenDB(opts)
		defer db.Close()

		v, dirty, err := readSchemaVersionImpl(context.Background(), db)
		require.NoError(t, err)
		assert.Equal(t, uint(0), v)
		assert.False(t, dirty)
	})

	t.Run("unexpected error returns error", func(t *testing.T) {
		srv := clickhousetest.NewServer(clickhousetest.FailureConfig{
			selectSchemaVersionQuery: errors.New("mock db failure"),
		})
		defer srv.Close()
		cfg := Configuration{
			Protocol:  "http",
			Addresses: []string{srv.Listener.Addr().String()},
		}
		opts, err := Options(context.Background(), cfg)
		require.NoError(t, err)
		db := clickhouse.OpenDB(opts)
		defer db.Close()

		_, _, err = readSchemaVersionImpl(context.Background(), db)
		require.ErrorContains(t, err, "failed to read database schema version")
		require.ErrorContains(t, err, "mock db failure")
	})
}

func TestLatestBinaryVersion(t *testing.T) {
	t.Run("first version error", func(t *testing.T) {
		drv := &mockSourceDriver{firstErr: errors.New("mock first error")}
		_, err := latestBinaryVersion(drv)
		require.ErrorContains(t, err, "mock first error")
	})

	t.Run("next version error", func(t *testing.T) {
		drv := &mockSourceDriver{
			firstVersion: 1,
			nextErr:      errors.New("mock next error"),
		}
		_, err := latestBinaryVersion(drv)
		require.ErrorContains(t, err, "mock next error")
	})

	t.Run("multiple versions", func(t *testing.T) {
		drv := &mockSourceDriver{
			firstVersion: 1,
			nextFn: func(v uint) (uint, error) {
				if v == 1 {
					return 2, nil
				}
				return 0, os.ErrNotExist
			},
		}
		v, err := latestBinaryVersion(drv)
		require.NoError(t, err)
		assert.Equal(t, uint(2), v)
	})

	t.Run("no migrations", func(t *testing.T) {
		drv := &mockSourceDriver{firstErr: os.ErrNotExist}
		v, err := latestBinaryVersion(drv)
		require.NoError(t, err)
		assert.Equal(t, uint(0), v)
	})
}

type mockSourceDriver struct {
	firstVersion uint
	firstErr     error
	nextVersion  uint
	nextErr      error
	nextFn       func(uint) (uint, error)
}

func (m *mockSourceDriver) Open(_ string) (source.Driver, error) { return m, nil }
func (*mockSourceDriver) Close() error                           { return nil }
func (m *mockSourceDriver) First() (uint, error)                 { return m.firstVersion, m.firstErr }
func (*mockSourceDriver) Prev(_ uint) (uint, error)              { return 0, os.ErrNotExist }
func (m *mockSourceDriver) Next(v uint) (uint, error) {
	if m.nextFn != nil {
		return m.nextFn(v)
	}
	return m.nextVersion, m.nextErr
}

func (*mockSourceDriver) ReadUp(_ uint) (io.ReadCloser, string, error) {
	return nil, "", os.ErrNotExist
}

func (*mockSourceDriver) ReadDown(_ uint) (io.ReadCloser, string, error) {
	return nil, "", os.ErrNotExist
}

func TestSchemaBuilder_ConcurrencyAndRetry(t *testing.T) {
	srv := clickhousetest.NewServer(clickhousetest.FailureConfig{})
	defer srv.Close()

	cfg := Configuration{
		Protocol:     "http",
		Addresses:    []string{srv.Listener.Addr().String()},
		CreateSchema: true,
		TableEngine:  localTableEngine,
	}
	opts, err := Options(context.Background(), cfg)
	require.NoError(t, err)

	t.Run("retry on ErrDirty succeeds when cleared", func(t *testing.T) {
		b, err := newSchemaBuilder(cfg, opts)
		require.NoError(t, err)
		b.backoffInitialInterval = 1 * time.Millisecond
		b.backoffMaxElapsedTime = 200 * time.Millisecond

		attempts := 0
		orig := newDatabaseDriver
		defer func() { newDatabaseDriver = orig }()
		newDatabaseDriver = func(db *dbsql.DB, config *clickhousemigrate.Config) (database.Driver, error) {
			drv, err := orig(db, config)
			require.NoError(t, err)
			return &mockFlakyDriver{
				Driver: drv,
				versionFn: func() (int, bool, error) {
					attempts++
					if attempts == 1 {
						return 1, true, nil // dirty!
					}
					return 1, false, nil // clean on second try!
				},
			}, nil
		}

		err = b.build(context.Background())
		require.NoError(t, err)
		assert.Equal(t, 2, attempts)
	})

	t.Run("retry on ErrDirty times out after bounded backoff", func(t *testing.T) {
		b, err := newSchemaBuilder(cfg, opts)
		require.NoError(t, err)
		b.backoffInitialInterval = 1 * time.Millisecond
		b.backoffMaxElapsedTime = 10 * time.Millisecond

		orig := newDatabaseDriver
		defer func() { newDatabaseDriver = orig }()
		newDatabaseDriver = func(db *dbsql.DB, config *clickhousemigrate.Config) (database.Driver, error) {
			drv, err := orig(db, config)
			require.NoError(t, err)
			return &mockFlakyDriver{
				Driver: drv,
				versionFn: func() (int, bool, error) {
					return 1, true, nil // always dirty!
				},
			}, nil
		}

		err = b.build(context.Background())
		require.ErrorContains(t, err, "ClickHouse schema_migrations table is dirty at version 1. Inspect the table and clean state using the migrate CLI (migrate force 1)")
	})

	t.Run("context canceled during retry backoff", func(t *testing.T) {
		b, err := newSchemaBuilder(cfg, opts)
		require.NoError(t, err)
		b.backoffInitialInterval = 50 * time.Millisecond
		b.backoffMaxElapsedTime = 5 * time.Second

		orig := newDatabaseDriver
		defer func() { newDatabaseDriver = orig }()
		newDatabaseDriver = func(db *dbsql.DB, config *clickhousemigrate.Config) (database.Driver, error) {
			drv, err := orig(db, config)
			require.NoError(t, err)
			return &mockFlakyDriver{
				Driver: drv,
				versionFn: func() (int, bool, error) {
					return 1, true, nil // dirty
				},
			}, nil
		}

		ctx, cancel := context.WithCancel(context.Background())
		go func() {
			time.Sleep(10 * time.Millisecond)
			cancel()
		}()

		err = b.build(ctx)
		require.ErrorIs(t, err, context.Canceled)
	})
}

type mockFlakyDriver struct {
	database.Driver
	versionFn func() (int, bool, error)
}

func (m *mockFlakyDriver) Version() (int, bool, error) {
	if m.versionFn != nil {
		return m.versionFn()
	}
	return m.Driver.Version()
}

func TestSchemaBuilder_Build(t *testing.T) {
	srv := clickhousetest.NewServer(clickhousetest.FailureConfig{})
	defer srv.Close()

	cfg := Configuration{
		Protocol:     "http",
		Addresses:    []string{srv.Listener.Addr().String()},
		CreateSchema: true,
		TableEngine:  localTableEngine,
	}
	opts, err := Options(context.Background(), cfg)
	require.NoError(t, err)

	t.Run("source driver creation error", func(t *testing.T) {
		b, err := newSchemaBuilder(cfg, opts)
		require.NoError(t, err)

		orig := newSourceDriver
		defer func() { newSourceDriver = orig }()
		newSourceDriver = func(_ fs.FS, _ string) (source.Driver, error) {
			return nil, errors.New("mock source error")
		}

		err = b.build(context.Background())
		require.ErrorContains(t, err, "failed to create migration source driver")
	})

	t.Run("latestBinaryVersion error", func(t *testing.T) {
		b, err := newSchemaBuilder(cfg, opts)
		require.NoError(t, err)

		orig := newSourceDriver
		defer func() { newSourceDriver = orig }()
		newSourceDriver = func(_ fs.FS, _ string) (source.Driver, error) {
			return &mockSourceDriver{firstErr: errors.New("mock binary version error")}, nil
		}

		err = b.build(context.Background())
		require.ErrorContains(t, err, "failed to determine binary schema version")
	})

	t.Run("database driver creation error", func(t *testing.T) {
		b, err := newSchemaBuilder(cfg, opts)
		require.NoError(t, err)

		orig := newDatabaseDriver
		defer func() { newDatabaseDriver = orig }()
		newDatabaseDriver = func(_ *dbsql.DB, _ *clickhousemigrate.Config) (database.Driver, error) {
			return nil, errors.New("mock db driver error")
		}

		err = b.build(context.Background())
		require.ErrorContains(t, err, "failed to create migration database driver")
	})

	t.Run("migrate instance creation error", func(t *testing.T) {
		b, err := newSchemaBuilder(cfg, opts)
		require.NoError(t, err)

		orig := newMigrateInstance
		defer func() { newMigrateInstance = orig }()
		newMigrateInstance = func(_ string, _ source.Driver, _ string, _ database.Driver) (*migrate.Migrate, error) {
			return nil, errors.New("mock migrate instance error")
		}

		err = b.build(context.Background())
		require.ErrorContains(t, err, "failed to create migrate instance")
	})

	t.Run("replicated table engine passes MigrationsTableEngine", func(t *testing.T) {
		repCfg := Configuration{
			Protocol:     "http",
			Addresses:    []string{srv.Listener.Addr().String()},
			CreateSchema: true,
			TableEngine: TableEngine{
				Replicated: configoptional.Some(ReplicatedEngine{
					KeeperPath:  "/clickhouse/tables/{shard}/{database}/{table}",
					ReplicaName: "{replica}",
				}),
			},
		}
		repOpts, err := Options(context.Background(), repCfg)
		require.NoError(t, err)

		b, err := newSchemaBuilder(repCfg, repOpts)
		require.NoError(t, err)

		var capturedEngine string
		orig := newDatabaseDriver
		defer func() { newDatabaseDriver = orig }()
		newDatabaseDriver = func(db *dbsql.DB, config *clickhousemigrate.Config) (database.Driver, error) {
			capturedEngine = config.MigrationsTableEngine
			return orig(db, config)
		}

		err = b.build(context.Background())
		require.NoError(t, err)
		assert.Equal(t, "ReplicatedMergeTree('/clickhouse/tables/{shard}/{database}/{table}', '{replica}') ORDER BY sequence", capturedEngine)
	})

	t.Run("replicated table engine default passes MigrationsTableEngine", func(t *testing.T) {
		repCfg := Configuration{
			Protocol:     "http",
			Addresses:    []string{srv.Listener.Addr().String()},
			CreateSchema: true,
			TableEngine: TableEngine{
				Replicated: configoptional.Some(ReplicatedEngine{}),
			},
		}
		repOpts, err := Options(context.Background(), repCfg)
		require.NoError(t, err)

		b, err := newSchemaBuilder(repCfg, repOpts)
		require.NoError(t, err)

		var capturedEngine string
		orig := newDatabaseDriver
		defer func() { newDatabaseDriver = orig }()
		newDatabaseDriver = func(db *dbsql.DB, config *clickhousemigrate.Config) (database.Driver, error) {
			capturedEngine = config.MigrationsTableEngine
			return orig(db, config)
		}

		err = b.build(context.Background())
		require.NoError(t, err)
		assert.Equal(t, "ReplicatedMergeTree", capturedEngine)
	})

	t.Run("create_schema false checks database version against binary version", func(t *testing.T) {
		noCreateCfg := Configuration{
			Protocol:     "http",
			Addresses:    []string{srv.Listener.Addr().String()},
			CreateSchema: false,
		}
		noCreateOpts, err := Options(context.Background(), noCreateCfg)
		require.NoError(t, err)

		b, err := newSchemaBuilder(noCreateCfg, noCreateOpts)
		require.NoError(t, err)

		err = b.build(context.Background())
		require.NoError(t, err)
	})

	t.Run("create_schema false fails when database version newer than binary", func(t *testing.T) {
		noCreateCfg := Configuration{
			Protocol:     "http",
			Addresses:    []string{srv.Listener.Addr().String()},
			CreateSchema: false,
		}
		noCreateOpts, err := Options(context.Background(), noCreateCfg)
		require.NoError(t, err)

		b, err := newSchemaBuilder(noCreateCfg, noCreateOpts)
		require.NoError(t, err)

		orig := readSchemaVersion
		defer func() { readSchemaVersion = orig }()
		readSchemaVersion = func(_ context.Context, _ *dbsql.DB) (uint, bool, error) {
			return 999, false, nil
		}

		err = b.build(context.Background())
		require.ErrorContains(t, err, "is newer than binary version")
	})

	t.Run("create_schema false fails when dirty", func(t *testing.T) {
		noCreateCfg := Configuration{
			Protocol:     "http",
			Addresses:    []string{srv.Listener.Addr().String()},
			CreateSchema: false,
		}
		noCreateOpts, err := Options(context.Background(), noCreateCfg)
		require.NoError(t, err)

		b, err := newSchemaBuilder(noCreateCfg, noCreateOpts)
		require.NoError(t, err)

		orig := readSchemaVersion
		defer func() { readSchemaVersion = orig }()
		readSchemaVersion = func(_ context.Context, _ *dbsql.DB) (uint, bool, error) {
			return 1, true, nil
		}

		err = b.build(context.Background())
		require.ErrorContains(t, err, "database schema is in dirty state at version 1")
	})

	t.Run("create_schema false fails when readSchemaVersion fails", func(t *testing.T) {
		noCreateCfg := Configuration{
			Protocol:     "http",
			Addresses:    []string{srv.Listener.Addr().String()},
			CreateSchema: false,
		}
		noCreateOpts, err := Options(context.Background(), noCreateCfg)
		require.NoError(t, err)

		b, err := newSchemaBuilder(noCreateCfg, noCreateOpts)
		require.NoError(t, err)

		orig := readSchemaVersion
		defer func() { readSchemaVersion = orig }()
		readSchemaVersion = func(_ context.Context, _ *dbsql.DB) (uint, bool, error) {
			return 0, false, errors.New("mock version error")
		}

		err = b.build(context.Background())
		require.ErrorContains(t, err, "mock version error")
	})
}

func TestNewFactory_Lifecycle(t *testing.T) {
	srv := clickhousetest.NewServer(clickhousetest.FailureConfig{})
	defer srv.Close()

	cfg := Configuration{
		Protocol:     "http",
		Addresses:    []string{srv.Listener.Addr().String()},
		CreateSchema: false,
	}

	t.Run("options error fails factory creation", func(t *testing.T) {
		badCfg := cfg
		badCfg.TLS = configoptional.Some(configtls.ClientConfig{
			Config: configtls.Config{
				CAFile: "/nonexistent/ca.pem",
			},
		})
		_, err := NewFactory(context.Background(), badCfg, telemetry.NoopSettings())
		require.Error(t, err)
	})

	t.Run("ping failure fails factory creation", func(t *testing.T) {
		failSrv := clickhousetest.NewServer(clickhousetest.FailureConfig{
			clickhousetest.PingQuery: errors.New("mock ping error"),
		})
		defer failSrv.Close()

		failCfg := cfg
		failCfg.Addresses = []string{failSrv.Listener.Addr().String()}
		_, err := NewFactory(context.Background(), failCfg, telemetry.NoopSettings())
		require.ErrorContains(t, err, "failed to ping ClickHouse")
	})

	t.Run("build failure closes connection and returns error", func(t *testing.T) {
		failCfg := cfg
		failCfg.CreateSchema = true
		failCfg.TableEngine = localTableEngine

		orig := newSourceDriver
		defer func() { newSourceDriver = orig }()
		newSourceDriver = func(_ fs.FS, _ string) (source.Driver, error) {
			return nil, errors.New("mock build failure")
		}

		_, err := NewFactory(context.Background(), failCfg, telemetry.NoopSettings())
		require.ErrorContains(t, err, "mock build failure")
	})

	t.Run("factory success and create readers/writers and purge", func(t *testing.T) {
		f, err := NewFactory(context.Background(), cfg, telemetry.NoopSettings())
		require.NoError(t, err)
		defer func() { require.NoError(t, f.Close()) }()

		tr, err := f.CreateTraceReader()
		require.NoError(t, err)
		assert.NotNil(t, tr)

		tw, err := f.CreateTraceWriter()
		require.NoError(t, err)
		assert.NotNil(t, tw)

		dr, err := f.CreateDependencyReader()
		require.NoError(t, err)
		assert.NotNil(t, dr)

		dw, err := f.CreateDependencyWriter()
		require.NoError(t, err)
		assert.NotNil(t, dw)

		mr, err := f.CreateMetricsReader()
		require.NoError(t, err)
		assert.NotNil(t, mr)

		err = f.Purge(context.Background())
		require.NoError(t, err)
	})
}
