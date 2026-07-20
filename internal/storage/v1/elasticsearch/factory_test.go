// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package elasticsearch

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/config/configoptional"
	"go.opentelemetry.io/collector/extension/extensionauth"
	"go.opentelemetry.io/otel"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"

	"github.com/jaegertracing/jaeger/internal/metrics"
	es "github.com/jaegertracing/jaeger/internal/storage/elasticsearch"
	escfg "github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/esclient"
	esdepstorev2 "github.com/jaegertracing/jaeger/internal/storage/v2/elasticsearch/depstore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/elasticsearch/tracestore/core"
	"github.com/jaegertracing/jaeger/internal/storage/v2/elasticsearch/tracestore/core/dbmodel"
)

var mockEsServerResponse = []byte(`
{
	"version": {
		"number": "7.10.2"
	},
	"tagline": "You Know, for Search"
}
`)

func TestElasticsearchFactoryBase(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write(mockEsServerResponse)
	}))
	t.Cleanup(server.Close)
	cfg := escfg.Configuration{
		Servers:  []string{server.URL},
		LogLevel: "debug",
	}
	f, err := NewFactoryBase(context.Background(), cfg, metrics.NullFactory, zaptest.NewLogger(t), nil)
	require.NoError(t, err)
	readerParams := f.GetSpanReaderParams()
	assert.IsType(t, core.SpanReaderParams{}, readerParams)
	writerParams := f.GetSpanWriterParams()
	assert.IsType(t, core.SpanWriterParams{}, writerParams)
	depParams := f.GetDependencyStoreParams()
	assert.IsType(t, esdepstorev2.Params{}, depParams)
	_, err = f.CreateSamplingStore(1)
	require.NoError(t, err)
	require.NoError(t, f.Close())
}

func TestFactoryBase_Purge(t *testing.T) {
	tests := []struct {
		name        string
		status      int
		expectedErr bool
	}{
		{name: "successful purge", status: http.StatusOK, expectedErr: false},
		{name: "purge error", status: http.StatusInternalServerError, expectedErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// The handler runs on a separate goroutine, so guard the captured
			// request fields with a mutex.
			var (
				mu                           sync.Mutex
				gotMethod, gotPath, gotQuery string
			)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodDelete {
					mu.Lock()
					gotMethod, gotPath, gotQuery = r.Method, r.URL.Path, r.URL.RawQuery
					mu.Unlock()
					w.WriteHeader(tt.status)
					w.Write([]byte("{}"))
					return
				}
				w.Write(mockEsServerResponse)
			}))
			defer server.Close()

			esClient, err := esclient.NewClient(context.Background(),
				&escfg.Configuration{Servers: []string{server.URL}, Version: uint(es.ElasticV7)}, zap.NewNop(), nil)
			require.NoError(t, err)
			f := &FactoryBase{esClient: esClient, logger: zap.NewNop(), config: &escfg.Configuration{}}

			err = f.Purge(context.Background())
			mu.Lock()
			method, path, query := gotMethod, gotPath, gotQuery
			mu.Unlock()
			if tt.expectedErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, http.MethodDelete, method)
				assert.Equal(t, "/*", path)
				// Cleanup tolerates missing indices, and no master_timeout=0s is
				// sent (the cluster default is used instead).
				assert.Contains(t, query, "ignore_unavailable=true")
				assert.NotContains(t, query, "master_timeout")
			}
		})
	}
}

func TestElasticsearchTagsFileDoNotExist(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write(mockEsServerResponse)
	}))
	t.Cleanup(server.Close)
	cfg := escfg.Configuration{
		Servers: []string{server.URL},
		Tags: escfg.TagsAsFields{
			File: "fixtures/file-does-not-exist.txt",
		},
		LogLevel: "debug",
	}
	f, err := NewFactoryBase(context.Background(), cfg, metrics.NullFactory, zaptest.NewLogger(t), nil)
	require.ErrorContains(t, err, "open fixtures/file-does-not-exist.txt: no such file or directory")
	assert.Nil(t, f)
}

func TestTagKeysAsFields(t *testing.T) {
	dir := t.TempDir()

	tagsFile := filepath.Join(dir, "tags.txt")
	require.NoError(t, os.WriteFile(tagsFile, []byte("foo\nbar\n      space   \n"), 0o600))

	emptyFile := filepath.Join(dir, "empty.txt")
	require.NoError(t, os.WriteFile(emptyFile, []byte(""), 0o600))

	missingFile := filepath.Join(dir, "missing.txt") // intentionally not created

	tests := []struct {
		path          string
		include       string
		expected      []string
		errorExpected bool
	}{
		{
			path:          missingFile,
			errorExpected: true,
		},
		{
			path:     tagsFile,
			expected: []string{"foo", "bar", "space"},
		},
		{
			path:     emptyFile,
			expected: nil,
		},
		{
			include:  "televators,eriatarka,thewidow",
			expected: []string{"televators", "eriatarka", "thewidow"},
		},
		{
			expected: nil,
		},
		{
			path:     tagsFile,
			include:  "televators,eriatarka,thewidow",
			expected: []string{"foo", "bar", "space", "televators", "eriatarka", "thewidow"},
		},
		{
			path:     emptyFile,
			include:  "televators,eriatarka,thewidow",
			expected: []string{"televators", "eriatarka", "thewidow"},
		},
	}

	for _, test := range tests {
		cfg := escfg.Configuration{
			Tags: escfg.TagsAsFields{
				File:    test.path,
				Include: test.include,
			},
		}

		tags, err := cfg.TagKeysAsFields()
		if test.errorExpected {
			require.Error(t, err)
			assert.Nil(t, tags)
		} else {
			require.NoError(t, err)
			assert.Equal(t, test.expected, tags)
		}
	}
}

// TestCreateTemplates drives the migrated data-plane path: createTemplates
// installs the span and service templates through the owned esclient, so the
// test records the PUTs against a real server.
func TestCreateTemplates(t *testing.T) {
	tests := []struct {
		name      string
		status    int
		expectErr string
	}{
		{name: "success", status: http.StatusOK},
		{name: "template error", status: http.StatusInternalServerError, expectErr: "failed to create template"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var puts []string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPut {
					puts = append(puts, r.URL.Path)
					w.WriteHeader(test.status)
					w.Write([]byte("{}"))
					return
				}
				w.Write(mockEsServerResponse)
			}))
			defer server.Close()

			esClient, err := esclient.NewClient(
				context.Background(),
				&escfg.Configuration{Servers: []string{server.URL}, Version: uint(es.ElasticV7)},
				zap.NewNop(), nil,
			)
			require.NoError(t, err)
			f := &FactoryBase{
				esClient: esClient,
				logger:   zap.NewNop(),
				config: &escfg.Configuration{
					CreateIndexTemplates: true,
					Indices: escfg.Indices{
						Spans:    escfg.IndexOptions{Shards: 3, Replicas: new(int64(1))},
						Services: escfg.IndexOptions{Shards: 3, Replicas: new(int64(1))},
					},
				},
			}
			err = f.createTemplates(context.Background())
			if test.expectErr != "" {
				require.ErrorContains(t, err, test.expectErr)
			} else {
				require.NoError(t, err)
				assert.Equal(t, []string{"/_template/jaeger-span", "/_template/jaeger-service"}, puts)
			}
		})
	}
}

// TestCreateTemplatesServiceError exercises the service-template error branch:
// the span PUT succeeds and only the service PUT fails.
func TestCreateTemplatesServiceError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			if strings.Contains(r.URL.Path, escfg.ServiceIndexName) {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			w.Write([]byte("{}"))
			return
		}
		w.Write(mockEsServerResponse)
	}))
	defer server.Close()

	esClient, err := esclient.NewClient(context.Background(),
		&escfg.Configuration{Servers: []string{server.URL}, Version: uint(es.ElasticV7)}, zap.NewNop(), nil)
	require.NoError(t, err)
	f := &FactoryBase{
		esClient: esClient,
		logger:   zap.NewNop(),
		config: &escfg.Configuration{
			CreateIndexTemplates: true,
			Indices: escfg.Indices{
				Spans:    escfg.IndexOptions{Shards: 3, Replicas: new(int64(1))},
				Services: escfg.IndexOptions{Shards: 3, Replicas: new(int64(1))},
			},
		},
	}
	require.ErrorContains(t, f.createTemplates(context.Background()), escfg.ServiceIndexName)
}

// TestCreateSamplingStoreTemplateError exercises the sampling-template error
// branch in CreateSamplingStore.
func TestCreateSamplingStoreTemplateError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Write(mockEsServerResponse)
	}))
	defer server.Close()

	esClient, err := esclient.NewClient(context.Background(),
		&escfg.Configuration{Servers: []string{server.URL}, Version: uint(es.ElasticV7)}, zap.NewNop(), nil)
	require.NoError(t, err)
	f := &FactoryBase{
		esClient: esClient,
		logger:   zap.NewNop(),
		config: &escfg.Configuration{
			CreateIndexTemplates: true,
			Indices: escfg.Indices{
				Sampling: escfg.IndexOptions{Shards: 3, Replicas: new(int64(1))},
			},
		},
	}
	_, err = f.CreateSamplingStore(1)
	require.ErrorContains(t, err, "failed to create template")
}

func TestESStorageFactoryWithConfig(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write(mockEsServerResponse)
	}))
	defer server.Close()
	cfg := escfg.Configuration{
		Servers:  []string{server.URL},
		LogLevel: "error",
	}
	factory, err := NewFactoryBase(context.Background(), cfg, metrics.NullFactory, zap.NewNop(), nil)
	require.NoError(t, err)
	factory.Close()
}

func TestESStorageFactoryWithConfigError(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	}))
	defer server.Close()
	cfg := escfg.Configuration{
		Servers:            []string{server.URL},
		DisableHealthCheck: true,
		LogLevel:           "error",
	}
	_, err := NewFactoryBase(context.Background(), cfg, metrics.NullFactory, zap.NewNop(), nil)
	require.ErrorContains(t, err, "failed to create Elasticsearch data client")
}

// TestESStorageFactoryClosesOnTemplateError drives NewFactoryBase past client
// construction and fails at template creation, exercising the error return and
// the deferred cleanup that closes the already-built client and bulk indexer.
func TestESStorageFactoryClosesOnTemplateError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut { // template creation fails
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Write(mockEsServerResponse) // version ping succeeds
	}))
	defer server.Close()
	cfg := escfg.Configuration{
		Servers:              []string{server.URL},
		CreateIndexTemplates: true,
		DisableHealthCheck:   true,
		LogLevel:             "error",
		Indices: escfg.Indices{
			Spans:    escfg.IndexOptions{Shards: 1, Replicas: new(int64(0)), Priority: 10},
			Services: escfg.IndexOptions{Shards: 1, Replicas: new(int64(0)), Priority: 10},
		},
	}
	_, err := NewFactoryBase(context.Background(), cfg, metrics.NullFactory, zap.NewNop(), nil)
	require.Error(t, err)
}

func withESClientFn(fn func(context.Context, *escfg.Configuration, *zap.Logger, extensionauth.HTTPClient) (*esclient.Client, error)) factoryOption {
	return func(f *FactoryBase) { f.newESClientFn = fn }
}

func withBulkIndexerFn(fn func(*esclient.Client, esclient.BulkIndexerConfig, metrics.Factory, *zap.Logger) (*esclient.BulkIndexer, error)) factoryOption {
	return func(f *FactoryBase) { f.newBulkIndexerFn = fn }
}

// TestNewFactoryBaseDataClientError injects a failing esclient constructor to
// exercise the data-client error path and its deferred cleanup.
func TestNewFactoryBaseDataClientError(t *testing.T) {
	_, err := NewFactoryBase(
		context.Background(),
		escfg.Configuration{Servers: []string{"http://localhost:9200"}},
		metrics.NullFactory, zap.NewNop(), nil,
		withESClientFn(func(context.Context, *escfg.Configuration, *zap.Logger, extensionauth.HTTPClient) (*esclient.Client, error) {
			return nil, errors.New("data client boom")
		}),
	)
	require.ErrorContains(t, err, "data client")
}

// TestNewFactoryBaseBulkIndexerError injects a failing bulk-indexer constructor
// (the esclient succeeds) to exercise that error path and its deferred cleanup.
func TestNewFactoryBaseBulkIndexerError(t *testing.T) {
	_, err := NewFactoryBase(
		context.Background(),
		escfg.Configuration{Servers: []string{"http://localhost:9200"}},
		metrics.NullFactory, zap.NewNop(), nil,
		withESClientFn(func(context.Context, *escfg.Configuration, *zap.Logger, extensionauth.HTTPClient) (*esclient.Client, error) {
			return &esclient.Client{}, nil
		}),
		withBulkIndexerFn(func(*esclient.Client, esclient.BulkIndexerConfig, metrics.Factory, *zap.Logger) (*esclient.BulkIndexer, error) {
			return nil, errors.New("bulk boom")
		}),
	)
	require.ErrorContains(t, err, "bulk indexer")
}

func TestFactoryESClientsAreNil(t *testing.T) {
	f := &FactoryBase{}
	assert.NoError(t, f.Close()) // must not panic on nil client
}

func TestPasswordFromFile(t *testing.T) {
	runPasswordFromFileTest(t)
}

func runPasswordFromFileTest(t *testing.T) {
	const (
		pwd1  = "first password"
		pwd2  = "second password"
		upwd1 = "user:" + pwd1
		upwd2 = "user:" + pwd2
	)

	var authReceived sync.Map
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := strings.Split(r.Header.Get("Authorization"), " ")
		if !assert.Len(t, h, 2) {
			return
		}
		assert.Equal(t, "Basic", h[0])
		authBytes, err := base64.StdEncoding.DecodeString(h[1])
		assert.NoError(t, err, "header: %s", h)
		authReceived.Store(string(authBytes), struct{}{})
		w.Write(mockEsServerResponse)
	}))
	t.Cleanup(server.Close)

	pwdFile := filepath.Join(t.TempDir(), "pwd")
	require.NoError(t, os.WriteFile(pwdFile, []byte(pwd1), 0o600))

	cfg := escfg.Configuration{
		Servers:  []string{server.URL},
		LogLevel: "debug",
		Authentication: escfg.Authentication{
			BasicAuthentication: configoptional.Some(escfg.BasicAuthentication{
				Username:         "user",
				PasswordFilePath: pwdFile,
				ReloadInterval:   50 * time.Millisecond,
			}),
		},
		BulkProcessing: escfg.BulkProcessing{
			MaxBytes:      -1, // disable bulk size limit
			MaxActions:    -1, // disable bulk action limit
			FlushInterval: 10 * time.Millisecond,
		},
	}

	f, err := NewFactoryBase(context.Background(), cfg, metrics.NullFactory, zap.NewNop(), nil)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, f.Close()) })

	writer := core.NewSpanWriter(f.GetSpanWriterParams())
	require.NoError(t, writer.WriteSpans(context.Background(), []dbmodel.Span{{Process: dbmodel.Process{ServiceName: "foo"}}}))

	assert.Eventually(t, func() bool {
		_, ok := authReceived.Load(upwd1)
		return ok
	}, 5*time.Second, time.Millisecond, "expecting ES client to use first password")

	// Replace the password file atomically (same pattern as Kubernetes secret rotation)
	newPwdFile := filepath.Join(t.TempDir(), "pwd2")
	require.NoError(t, os.WriteFile(newPwdFile, []byte(pwd2), 0o600))
	require.NoError(t, os.Rename(newPwdFile, pwdFile))

	// After ReloadInterval expires the transport re-reads the file; keep writing
	// spans until the new auth header is observed.
	assert.Eventually(t, func() bool {
		// Eventually runs this condition on a separate goroutine, so a write
		// error is surfaced by returning false (retry) rather than require.
		if err := writer.WriteSpans(context.Background(), []dbmodel.Span{{Process: dbmodel.Process{ServiceName: "foo"}}}); err != nil {
			return false
		}
		_, ok := authReceived.Load(upwd2)
		return ok
	}, 5*time.Second, 100*time.Millisecond, "expecting ES client to use second password after cache reload")
}

// TestFactoryBase_MissingPasswordFile verifies that factory creation fails fast
// when a PasswordFilePath is configured but the file does not exist.
func TestFactoryBase_MissingPasswordFile(t *testing.T) {
	cfg := escfg.Configuration{
		Servers:  []string{"http://localhost:9200"},
		LogLevel: "debug",
		Authentication: escfg.Authentication{
			BasicAuthentication: configoptional.Some(escfg.BasicAuthentication{
				Username:         "testuser",
				PasswordFilePath: "/nonexistent/path/to/password.txt",
			}),
		},
	}

	_, err := NewFactoryBase(context.Background(), cfg, metrics.NullFactory, zaptest.NewLogger(t), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to initialize basic authentication")
	assert.Contains(t, err.Error(), "failed to get token from file")
}

func TestElasticsearchFactoryBaseWithAuthenticator(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write(mockEsServerResponse)
	}))
	t.Cleanup(server.Close)

	cfg := escfg.Configuration{
		Servers:  []string{server.URL},
		LogLevel: "debug",
		BulkProcessing: escfg.BulkProcessing{
			MaxBytes:   -1, // disable bulk
			MaxActions: -1, // disable bulk; the test only validates authenticator setup
		},
	}

	mockAuth := &mockHTTPAuthenticator{}

	f, err := NewFactoryBase(context.Background(), cfg, metrics.NullFactory, zaptest.NewLogger(t), mockAuth)
	require.NoError(t, err)
	require.NotNil(t, f)
	defer require.NoError(t, f.Close())

	readerParams := f.GetSpanReaderParams()
	assert.IsType(t, core.SpanReaderParams{}, readerParams)
}

func TestBuildRotations(t *testing.T) {
	date := time.Date(2019, 10, 10, 5, 0, 0, 0, time.UTC)
	spanDataLayout := "2006-01-02-15"
	serviceDataLayout := "2006-01-02"
	spanDataLayoutFormat := date.UTC().Format(spanDataLayout)
	serviceDataLayoutFormat := date.UTC().Format(serviceDataLayout)

	testCases := []struct {
		name         string
		cfg          escfg.Configuration
		readIndices  []string
		writeIndices []string
	}{
		{
			name: "periodic rotation",
			cfg: escfg.Configuration{
				Indices: escfg.Indices{
					Spans:    escfg.IndexOptions{DateLayout: configoptional.Some(spanDataLayout)},
					Services: escfg.IndexOptions{DateLayout: configoptional.Some(serviceDataLayout)},
				},
			},
			readIndices:  []string{"jaeger-span-" + spanDataLayoutFormat, "jaeger-service-" + serviceDataLayoutFormat},
			writeIndices: []string{"jaeger-span-" + spanDataLayoutFormat, "jaeger-service-" + serviceDataLayoutFormat},
		},
		{
			name: "alias rotation",
			cfg: escfg.Configuration{
				UseReadWriteAliases: configoptional.Some(true),
			},
			readIndices:  []string{"jaeger-span-read", "jaeger-service-read"},
			writeIndices: []string{"jaeger-span-write", "jaeger-service-write"},
		},
		{
			name: "alias with custom suffixes",
			cfg: escfg.Configuration{
				UseReadWriteAliases: configoptional.Some(true),
				ReadAliasSuffix:     "archive-read",
				WriteAliasSuffix:    "archive-write",
			},
			readIndices:  []string{"jaeger-span-archive-read", "jaeger-service-archive-read"},
			writeIndices: []string{"jaeger-span-archive-write", "jaeger-service-archive-write"},
		},
		{
			name: "explicit aliases",
			cfg: escfg.Configuration{
				SpanWriteAlias:    configoptional.Some("custom-span-write"),
				SpanReadAlias:     configoptional.Some("custom-span-read"),
				ServiceWriteAlias: configoptional.Some("custom-service-write"),
				ServiceReadAlias:  configoptional.Some("custom-service-read"),
			},
			readIndices:  []string{"custom-span-read", "custom-service-read"},
			writeIndices: []string{"custom-span-write", "custom-service-write"},
		},
		{
			name: "with index prefix",
			cfg: escfg.Configuration{
				Indices: escfg.Indices{
					IndexPrefix: "foo:",
					Spans:       escfg.IndexOptions{DateLayout: configoptional.Some(spanDataLayout)},
					Services:    escfg.IndexOptions{DateLayout: configoptional.Some(serviceDataLayout)},
				},
			},
			readIndices:  []string{"foo:-jaeger-span-" + spanDataLayoutFormat, "foo:-jaeger-service-" + serviceDataLayoutFormat},
			writeIndices: []string{"foo:-jaeger-span-" + spanDataLayoutFormat, "foo:-jaeger-service-" + serviceDataLayoutFormat},
		},
		{
			name: "with remote clusters",
			cfg: escfg.Configuration{
				Indices: escfg.Indices{
					Spans:    escfg.IndexOptions{DateLayout: configoptional.Some(spanDataLayout)},
					Services: escfg.IndexOptions{DateLayout: configoptional.Some(serviceDataLayout)},
				},
				RemoteReadClusters: []string{"cluster_one", "cluster_two"},
			},
			readIndices: []string{
				"jaeger-span-" + spanDataLayoutFormat,
				"cluster_one:jaeger-span-" + spanDataLayoutFormat,
				"cluster_two:jaeger-span-" + spanDataLayoutFormat,
				"jaeger-service-" + serviceDataLayoutFormat,
				"cluster_one:jaeger-service-" + serviceDataLayoutFormat,
				"cluster_two:jaeger-service-" + serviceDataLayoutFormat,
			},
			writeIndices: []string{"jaeger-span-" + spanDataLayoutFormat, "jaeger-service-" + serviceDataLayoutFormat},
		},
		{
			name: "rotation config: periodic",
			cfg: escfg.Configuration{
				Indices: escfg.Indices{
					Spans: escfg.IndexOptions{
						Rotation: escfg.RotationConfig{
							Periodic: configoptional.Some(escfg.PeriodicRotation{DateLayout: spanDataLayout}),
						},
					},
					Services: escfg.IndexOptions{
						Rotation: escfg.RotationConfig{
							Periodic: configoptional.Some(escfg.PeriodicRotation{DateLayout: serviceDataLayout}),
						},
					},
				},
			},
			readIndices:  []string{"jaeger-span-" + spanDataLayoutFormat, "jaeger-service-" + serviceDataLayoutFormat},
			writeIndices: []string{"jaeger-span-" + spanDataLayoutFormat, "jaeger-service-" + serviceDataLayoutFormat},
		},
		{
			name: "rotation config: manual_rollover",
			cfg: escfg.Configuration{
				Indices: escfg.Indices{
					Spans: escfg.IndexOptions{
						Rotation: escfg.RotationConfig{
							ManualRollover: configoptional.Some(escfg.ManualRolloverRotation{
								ReadAlias:  "my-span-read",
								WriteAlias: "my-span-write",
							}),
						},
					},
					Services: escfg.IndexOptions{
						Rotation: escfg.RotationConfig{
							ManualRollover: configoptional.Some(escfg.ManualRolloverRotation{
								ReadAlias:  "my-service-read",
								WriteAlias: "my-service-write",
							}),
						},
					},
				},
			},
			readIndices:  []string{"my-span-read", "my-service-read"},
			writeIndices: []string{"my-span-write", "my-service-write"},
		},
		{
			name: "rotation config: auto_rollover with defaults",
			cfg: escfg.Configuration{
				Indices: escfg.Indices{
					Spans: escfg.IndexOptions{
						Rotation: escfg.RotationConfig{
							AutoRollover: configoptional.Some(escfg.AutoRolloverRotation{}),
						},
					},
					Services: escfg.IndexOptions{
						Rotation: escfg.RotationConfig{
							AutoRollover: configoptional.Some(escfg.AutoRolloverRotation{}),
						},
					},
				},
			},
			readIndices:  []string{"jaeger-span-read", "jaeger-service-read"},
			writeIndices: []string{"jaeger-span-write", "jaeger-service-write"},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			f := &FactoryBase{config: &tc.cfg, logger: zap.NewNop()}
			spanRotation, serviceRotation := f.buildRotations()
			actualRead := append(spanRotation.ReadTargets(date, date), serviceRotation.ReadTargets(date, date)...)
			assert.Equal(t, tc.readIndices, actualRead)
			actualWrite := []string{spanRotation.WriteTarget(date), serviceRotation.WriteTarget(date)}
			assert.Equal(t, tc.writeIndices, actualWrite)
		})
	}
}

// TestIndicesClientFromConfig verifies the factory derives the template-rendering
// ILM inputs from the span rotation config: auto_rollover with a policy name
// enables ILM, while periodic rotation (or an empty policy name) leaves it off.
func TestIndicesClientFromConfig(t *testing.T) {
	tests := []struct {
		name               string
		cfg                escfg.Configuration
		expectedUseILM     bool
		expectedPolicyName string
	}{
		{
			name:           "periodic rotation - no ILM",
			cfg:            escfg.Configuration{},
			expectedUseILM: false,
		},
		{
			name: "auto_rollover with policy name",
			cfg: escfg.Configuration{
				Indices: escfg.Indices{
					Spans: escfg.IndexOptions{
						Rotation: escfg.RotationConfig{
							AutoRollover: configoptional.Some(escfg.AutoRolloverRotation{
								PolicyName: "my-policy",
							}),
						},
					},
				},
			},
			expectedUseILM:     true,
			expectedPolicyName: "my-policy",
		},
		{
			name: "auto_rollover without policy name",
			cfg: escfg.Configuration{
				Indices: escfg.Indices{
					Spans: escfg.IndexOptions{
						Rotation: escfg.RotationConfig{
							AutoRollover: configoptional.Some(escfg.AutoRolloverRotation{}),
						},
					},
				},
			},
			expectedUseILM: false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := &FactoryBase{config: &tc.cfg, logger: zap.NewNop()}
			ic := f.indicesClient()
			assert.Equal(t, tc.expectedUseILM, ic.UseILM)
			assert.Equal(t, tc.expectedPolicyName, ic.ILMPolicyName)
		})
	}
}

func TestGetSpanReaderParams_NonPeriodicMaxSpanAge(t *testing.T) {
	cfg := escfg.Configuration{
		Indices: escfg.Indices{
			Spans: escfg.IndexOptions{
				Rotation: escfg.RotationConfig{
					ManualRollover: configoptional.Some(escfg.ManualRolloverRotation{
						ReadAlias:  "span-read",
						WriteAlias: "span-write",
					}),
				},
			},
			Services: escfg.IndexOptions{
				Rotation: escfg.RotationConfig{
					ManualRollover: configoptional.Some(escfg.ManualRolloverRotation{
						ReadAlias:  "svc-read",
						WriteAlias: "svc-write",
					}),
				},
			},
		},
		MaxSpanAge: 72 * time.Hour,
	}
	f := &FactoryBase{config: &cfg, logger: zap.NewNop(), tracer: otel.GetTracerProvider()}
	params := f.GetSpanReaderParams()
	assert.Equal(t, core.DawnOfTimeSpanAge, params.MaxSpanAge)
}

func TestGetSpanReaderParams_MaxTraceDuration(t *testing.T) {
	cfg := escfg.Configuration{
		Indices: escfg.Indices{
			Spans: escfg.IndexOptions{
				Rotation: escfg.RotationConfig{
					Periodic: configoptional.Default(escfg.PeriodicRotation{
						DateLayout:        "2006-01-02",
						RolloverFrequency: "day",
					}),
				},
			},
			Services: escfg.IndexOptions{
				Rotation: escfg.RotationConfig{
					Periodic: configoptional.Default(escfg.PeriodicRotation{
						DateLayout:        "2006-01-02",
						RolloverFrequency: "day",
					}),
				},
			},
		},
		MaxSpanAge:       72 * time.Hour,
		MaxTraceDuration: 2 * time.Hour,
	}
	f := &FactoryBase{config: &cfg, logger: zap.NewNop(), tracer: otel.GetTracerProvider()}
	params := f.GetSpanReaderParams()
	assert.Equal(t, 72*time.Hour, params.MaxSpanAge)
	assert.Equal(t, 2*time.Hour, params.MaxTraceDuration)
}

// mockHTTPAuthenticator implements extensionauth.HTTPClient for testing
type mockHTTPAuthenticator struct{}

func (*mockHTTPAuthenticator) RoundTripper(base http.RoundTripper) (http.RoundTripper, error) {
	return &mockRoundTripper{base: base}, nil
}

// mockRoundTripper wraps the base RoundTripper
type mockRoundTripper struct {
	base http.RoundTripper
}

func (m *mockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Set("Authorization", "Bearer mock-token")
	if m.base != nil {
		return m.base.RoundTrip(req)
	}
	return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody}, nil
}
