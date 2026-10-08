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

	t.Run("TLS load error in newSchemaBuilder", func(t *testing.T) {
		tlsCfg := cfg
		tlsCfg.TLS = configoptional.Some(configtls.ClientConfig{
			Config: configtls.Config{
				CAFile: "/non/existent/ca.pem",
			},
		})
		_, err := newSchemaBuilder(context.Background(), tlsCfg)
		require.ErrorContains(t, err, "failed to load TLS configuration")
	})

	t.Run("source driver error", func(t *testing.T) {
		orig := newSourceDriver
		defer func() { newSourceDriver = orig }()
		newSourceDriver = func(_ fs.FS, _ string) (source.Driver, error) {
			return nil, errors.New("mock source driver error")
		}
		b, err := newSchemaBuilder(context.Background(), cfg, opts)
		require.NoError(t, err)
		err = b.build(context.Background())
		require.ErrorContains(t, err, "failed to create migration source driver: mock source driver error")
	})

	t.Run("database driver error", func(t *testing.T) {
		orig := newDatabaseDriver
		defer func() { newDatabaseDriver = orig }()
		newDatabaseDriver = func(_ *dbsql.DB, _ *clickhousemigrate.Config) (database.Driver, error) {
			return nil, errors.New("mock database driver error")
		}
		b, err := newSchemaBuilder(context.Background(), cfg, opts)
		require.NoError(t, err)
		err = b.build(context.Background())
		require.ErrorContains(t, err, "failed to create migration database driver: mock database driver error")
	})

	t.Run("migrate instance error", func(t *testing.T) {
		orig := newMigrateInstance
		defer func() { newMigrateInstance = orig }()
		newMigrateInstance = func(_ string, _ source.Driver, _ string, _ database.Driver) (*migrate.Migrate, error) {
			return nil, errors.New("mock migrate instance error")
		}
		b, err := newSchemaBuilder(context.Background(), cfg, opts)
		require.NoError(t, err)
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
		b, err := newSchemaBuilder(context.Background(), cfg, opts)
		require.NoError(t, err)

		// Set up mock driver where database version is 2, while binary is 1
		orig := newDatabaseDriver
		defer func() { newDatabaseDriver = orig }()
		newDatabaseDriver = func(db *dbsql.DB, config *clickhousemigrate.Config) (database.Driver, error) {
			drv, err := orig(db, config)
			require.NoError(t, err)
			return &mockVersionDriver{Driver: drv, version: 2}, nil
		}

		err = b.build(context.Background())
		require.ErrorContains(t, err, "database schema version 2 is newer than binary version 1")
	})

	t.Run("database version equal to binary allows startup", func(t *testing.T) {
		b, err := newSchemaBuilder(context.Background(), cfg, opts)
		require.NoError(t, err)

		orig := newDatabaseDriver
		defer func() { newDatabaseDriver = orig }()
		newDatabaseDriver = func(db *dbsql.DB, config *clickhousemigrate.Config) (database.Driver, error) {
			drv, err := orig(db, config)
			require.NoError(t, err)
			return &mockVersionDriver{Driver: drv, version: 1}, nil
		}

		err = b.build(context.Background())
		require.NoError(t, err)
	})

	t.Run("database version check error", func(t *testing.T) {
		b, err := newSchemaBuilder(context.Background(), cfg, opts)
		require.NoError(t, err)

		orig := newDatabaseDriver
		defer func() { newDatabaseDriver = orig }()
		newDatabaseDriver = func(db *dbsql.DB, config *clickhousemigrate.Config) (database.Driver, error) {
			drv, err := orig(db, config)
			require.NoError(t, err)
			return &mockVersionDriver{Driver: drv, err: errors.New("mock version error")}, nil
		}

		err = b.build(context.Background())
		require.ErrorContains(t, err, "failed to read database schema version")
	})
}

type mockVersionDriver struct {
	database.Driver
	version int
	dirty   bool
	err     error
}

func (m *mockVersionDriver) Version() (int, bool, error) {
	if m.err != nil {
		return 0, false, m.err
	}
	return m.version, m.dirty, nil
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
		b, err := newSchemaBuilder(context.Background(), cfg, opts)
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
		b, err := newSchemaBuilder(context.Background(), cfg, opts)
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
		require.ErrorContains(t, err, "schema migration dirty after backoff")
	})

	t.Run("context canceled during retry backoff", func(t *testing.T) {
		b, err := newSchemaBuilder(context.Background(), cfg, opts)
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
}

func (m *mockSourceDriver) Open(_ string) (source.Driver, error) { return m, nil }
func (*mockSourceDriver) Close() error                           { return nil }
func (m *mockSourceDriver) First() (uint, error)                 { return m.firstVersion, m.firstErr }
func (*mockSourceDriver) Prev(_ uint) (uint, error)              { return 0, os.ErrNotExist }
func (m *mockSourceDriver) Next(_ uint) (uint, error)            { return m.nextVersion, m.nextErr }
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
	t.Run("success", func(t *testing.T) {
		stmts, err := BaselineSchemaStatements()
		require.NoError(t, err)
		assert.Len(t, stmts, 12)
		assert.Len(t, SchemaObjects, 12)
		assert.Len(t, DropSchemaObjectsStatements, 13)
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
