// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package clickhouse

import (
	"context"
	dbsql "database/sql"
	"errors"
	"io"
	"io/fs"
	"os"
	"testing"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database"
	clickhousemigrate "github.com/golang-migrate/migrate/v4/database/clickhouse"
	"github.com/golang-migrate/migrate/v4/source"
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

			dw, err := f.CreateDependencyWriter()
			require.NoError(t, err)
			require.NotNil(t, dw)

			mr, err := f.CreateMetricsReader()
			require.NoError(t, err)
			require.NotNil(t, mr)

			err = f.Purge(context.Background())
			require.NoError(t, err)

			require.NoError(t, f.Close())
		})
	}
}

func TestNewFactory_Errors(t *testing.T) {
	servicesCreationQuery := `CREATE TABLE
    IF NOT EXISTS services (name String) ENGINE = AggregatingMergeTree
ORDER BY
    (name)`

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
			name: "migration execution error",
			failureConfig: clickhousetest.FailureConfig{
				servicesCreationQuery: assert.AnError,
			},
			expectedError: "failed to apply migrations",
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
	srv := clickhousetest.NewServer(clickhousetest.FailureConfig{})
	defer srv.Close()

	cfg := Configuration{
		Protocol:     "http",
		Addresses:    []string{srv.Listener.Addr().String()},
		CreateSchema: true,
	}
	opts, err := Options(context.Background(), cfg)
	require.NoError(t, err)

	t.Run("binary version determination error", func(t *testing.T) {
		orig := newSourceDriver
		defer func() { newSourceDriver = orig }()
		newSourceDriver = func(_ fs.FS, _ string) (source.Driver, error) {
			return &mockSourceDriver{firstErr: errors.New("mock binary version error")}, nil
		}
		b := newSchemaBuilder(cfg, opts)
		err := b.build(context.Background())
		require.ErrorContains(t, err, "failed to determine binary schema version: mock binary version error")
	})

	t.Run("source driver error", func(t *testing.T) {
		orig := newSourceDriver
		defer func() { newSourceDriver = orig }()
		newSourceDriver = func(_ fs.FS, _ string) (source.Driver, error) {
			return nil, errors.New("mock source driver error")
		}
		b := newSchemaBuilder(cfg, opts)
		err = b.build(context.Background())
		require.ErrorContains(t, err, "failed to create migration source driver: mock source driver error")
	})

	t.Run("database driver error", func(t *testing.T) {
		orig := newDatabaseDriver
		defer func() { newDatabaseDriver = orig }()
		newDatabaseDriver = func(_ *dbsql.DB, _ *clickhousemigrate.Config) (database.Driver, error) {
			return nil, errors.New("mock database driver error")
		}
		b := newSchemaBuilder(cfg, opts)
		err = b.build(context.Background())
		require.ErrorContains(t, err, "failed to create migration database driver: mock database driver error")
	})

	t.Run("migrate instance error", func(t *testing.T) {
		orig := newMigrateInstance
		defer func() { newMigrateInstance = orig }()
		newMigrateInstance = func(_ string, _ source.Driver, _ string, _ database.Driver) (*migrate.Migrate, error) {
			return nil, errors.New("mock migrate instance error")
		}
		b := newSchemaBuilder(cfg, opts)
		err = b.build(context.Background())
		require.ErrorContains(t, err, "failed to create migrate instance: mock migrate instance error")
	})
}

func TestSchemaBuilder_VersionChecking(t *testing.T) {
	srv := clickhousetest.NewServer(clickhousetest.FailureConfig{})
	defer srv.Close()

	cfg := Configuration{
		Protocol:     "http",
		Addresses:    []string{srv.Listener.Addr().String()},
		CreateSchema: false,
	}
	opts, err := Options(context.Background(), cfg)
	require.NoError(t, err)

	t.Run("database version newer than binary refuses startup", func(t *testing.T) {
		b := newSchemaBuilder(cfg, opts)

		orig := readSchemaVersion
		defer func() { readSchemaVersion = orig }()
		readSchemaVersion = func(_ context.Context, _ *dbsql.DB) (uint, bool, error) {
			return 2, false, nil
		}

		err = b.build(context.Background())
		require.ErrorContains(t, err, "database schema version 2 is newer than binary version 1")
	})

	t.Run("database version equal to binary allows startup", func(t *testing.T) {
		b := newSchemaBuilder(cfg, opts)

		orig := readSchemaVersion
		defer func() { readSchemaVersion = orig }()
		readSchemaVersion = func(_ context.Context, _ *dbsql.DB) (uint, bool, error) {
			return 1, false, nil
		}

		err = b.build(context.Background())
		require.NoError(t, err)
	})

	t.Run("dirty database schema refuses startup", func(t *testing.T) {
		b := newSchemaBuilder(cfg, opts)

		orig := readSchemaVersion
		defer func() { readSchemaVersion = orig }()
		readSchemaVersion = func(_ context.Context, _ *dbsql.DB) (uint, bool, error) {
			return 1, true, nil
		}

		err = b.build(context.Background())
		require.ErrorContains(t, err, "database schema is in dirty state at version 1")
	})

	t.Run("database version check error", func(t *testing.T) {
		b := newSchemaBuilder(cfg, opts)

		orig := readSchemaVersion
		defer func() { readSchemaVersion = orig }()
		readSchemaVersion = func(_ context.Context, _ *dbsql.DB) (uint, bool, error) {
			return 0, false, errors.New("mock version error")
		}

		err = b.build(context.Background())
		require.ErrorContains(t, err, "mock version error")
	})
}

func TestReadSchemaVersionImpl(t *testing.T) {
	t.Run("table does not exist returns version 0", func(t *testing.T) {
		srv := clickhousetest.NewServer(clickhousetest.FailureConfig{
			selectSchemaVersionQuery: errors.New("code: 60, message: Table default.schema_migrations doesn't exist"),
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

	t.Run("isTableNotExist", func(t *testing.T) {
		assert.False(t, isTableNotExist(nil))
		assert.True(t, isTableNotExist(&clickhouse.Exception{Code: 60}))
		assert.False(t, isTableNotExist(&clickhouse.Exception{Code: 59}))
		assert.True(t, isTableNotExist(errors.New("table not found")))
		assert.True(t, isTableNotExist(errors.New("Table default.schema_migrations does not exist")))
		assert.True(t, isTableNotExist(errors.New("unknown table: schema_migrations")))
		assert.False(t, isTableNotExist(errors.New("connection refused")))
	})
}

func TestSchemaBuilder_ApplyTTL(t *testing.T) {
	srv := clickhousetest.NewServer(clickhousetest.FailureConfig{})
	defer srv.Close()

	cfg := Configuration{
		Protocol:     "http",
		Addresses:    []string{srv.Listener.Addr().String()},
		CreateSchema: true,
		TTL:          24 * time.Hour,
	}
	opts, err := Options(context.Background(), cfg)
	require.NoError(t, err)

	t.Run("applies TTL when configured", func(t *testing.T) {
		b := newSchemaBuilder(cfg, opts)
		db := clickhouse.OpenDB(opts)
		defer db.Close()

		err := b.applyTTL(context.Background(), db)
		require.NoError(t, err)
	})

	t.Run("skips TTL when zero", func(t *testing.T) {
		zeroCfg := cfg
		zeroCfg.TTL = 0
		b := newSchemaBuilder(zeroCfg, opts)
		db := clickhouse.OpenDB(opts)
		defer db.Close()

		err := b.applyTTL(context.Background(), db)
		require.NoError(t, err)
	})

	t.Run("returns error when ALTER TABLE fails", func(t *testing.T) {
		failSrv := clickhousetest.NewServer(clickhousetest.FailureConfig{
			"ALTER TABLE spans MODIFY TTL": errors.New("mock alter error"),
		})
		defer failSrv.Close()
		failCfg := cfg
		failCfg.Addresses = []string{failSrv.Listener.Addr().String()}
		failOpts, err := Options(context.Background(), failCfg)
		require.NoError(t, err)
		b := newSchemaBuilder(failCfg, failOpts)
		db := clickhouse.OpenDB(failOpts)
		defer db.Close()

		err = b.applyTTL(context.Background(), db)
		require.ErrorContains(t, err, "failed to apply TTL")
		require.ErrorContains(t, err, "mock alter error")
	})

	t.Run("returns error in build when applyTTL fails", func(t *testing.T) {
		failSrv := clickhousetest.NewServer(clickhousetest.FailureConfig{
			"ALTER TABLE spans MODIFY TTL": errors.New("mock alter error"),
		})
		defer failSrv.Close()
		failCfg := cfg
		failCfg.Addresses = []string{failSrv.Listener.Addr().String()}
		failOpts, err := Options(context.Background(), failCfg)
		require.NoError(t, err)
		b := newSchemaBuilder(failCfg, failOpts)

		err = b.build(context.Background())
		require.ErrorContains(t, err, "failed to apply TTL")
		require.ErrorContains(t, err, "mock alter error")
	})
}

func TestSchemaBuilder_ConcurrencyAndRetry(t *testing.T) {
	srv := clickhousetest.NewServer(clickhousetest.FailureConfig{})
	defer srv.Close()

	cfg := Configuration{
		Protocol:     "http",
		Addresses:    []string{srv.Listener.Addr().String()},
		CreateSchema: true,
	}
	opts, err := Options(context.Background(), cfg)
	require.NoError(t, err)

	t.Run("retry on ErrDirty succeeds when cleared", func(t *testing.T) {
		b := newSchemaBuilder(cfg, opts)
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
		b := newSchemaBuilder(cfg, opts)
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
		require.ErrorContains(t, err, "schema migration dirty after backoff")
	})

	t.Run("context canceled during retry backoff", func(t *testing.T) {
		b := newSchemaBuilder(cfg, opts)
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
		queryWithoutTTL, err := loadTemplate("test_no_ttl", sql.CreateSpansTable, schemaTemplateParams{TTLSeconds: 0})
		require.NoError(t, err)
		assert.NotContains(t, queryWithoutTTL, "TTL start_time")
	})

	t.Run("with TTL", func(t *testing.T) {
		queryWithTTL, err := loadTemplate("test_ttl", sql.CreateSpansTable, schemaTemplateParams{TTLSeconds: 86400})
		require.NoError(t, err)
		assert.Contains(t, queryWithTTL, "TTL start_time + INTERVAL 86400 SECOND DELETE")
	})
}

func TestCreateTraceIDTimestampsTableTemplate(t *testing.T) {
	t.Run("without TTL", func(t *testing.T) {
		queryWithoutTTL, err := loadTemplate("test_no_ttl_trace", sql.CreateTraceIDTimestampsTable, schemaTemplateParams{TTLSeconds: 0})
		require.NoError(t, err)
		assert.NotContains(t, queryWithoutTTL, "TTL end")
	})

	t.Run("with TTL", func(t *testing.T) {
		queryWithTTL, err := loadTemplate("test_ttl_trace", sql.CreateTraceIDTimestampsTable, schemaTemplateParams{TTLSeconds: 86400})
		require.NoError(t, err)
		assert.Contains(t, queryWithTTL, "TTL end + INTERVAL 86400 SECOND DELETE")
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

func TestBaselineSchemaStatements(t *testing.T) {
	t.Run("success without TTL", func(t *testing.T) {
		stmts, err := BaselineSchemaStatements()
		require.NoError(t, err)
		assert.Len(t, stmts, 12)
		assert.Len(t, SchemaObjects, 12)
		assert.Len(t, DropSchemaObjectsStatements, 13)
		assert.NotContains(t, stmts[0], "TTL start_time")
		assert.NotContains(t, stmts[5], "TTL end")
	})

	t.Run("success with TTL", func(t *testing.T) {
		stmts, err := BaselineSchemaStatements(86400)
		require.NoError(t, err)
		assert.Len(t, stmts, 12)
		assert.Contains(t, stmts[0], "TTL start_time + INTERVAL 86400 SECOND DELETE")
		assert.Contains(t, stmts[5], "TTL end + INTERVAL 86400 SECOND DELETE")
	})

	t.Run("first template error", func(t *testing.T) {
		orig := loadTemplate
		defer func() { loadTemplate = orig }()
		loadTemplate = func(_, _ string, _ any) (string, error) {
			return "", errors.New("mock template error")
		}
		_, err := BaselineSchemaStatements()
		require.ErrorContains(t, err, "mock template error")
	})

	t.Run("second template error", func(t *testing.T) {
		orig := loadTemplate
		defer func() { loadTemplate = orig }()
		calls := 0
		loadTemplate = func(name, tmplBody string, data any) (string, error) {
			calls++
			if calls == 2 {
				return "", errors.New("mock template error 2")
			}
			return orig(name, tmplBody, data)
		}
		_, err := BaselineSchemaStatements()
		require.ErrorContains(t, err, "mock template error 2")
	})
}
