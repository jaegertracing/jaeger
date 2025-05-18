// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package elasticsearch

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"

	"github.com/jaegertracing/jaeger/internal/metrics"
	escfg "github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
	"github.com/jaegertracing/jaeger/internal/storage/v1/elasticsearch"
	"github.com/jaegertracing/jaeger/internal/telemetry"
)

var mockEsServerResponse = []byte(`
{
	"Version": {
		"Number": "6"
	}
}
`)

func TestNewFactory(t *testing.T) {
	cfg := &escfg.Configuration{}
	coreFactory := getTestingFactoryBase(t, cfg)
	f := &Factory{coreFactory: coreFactory, config: cfg}
	_, err := f.CreateTraceReader()
	require.NoError(t, err)
	_, err = f.CreateTraceWriter()
	require.NoError(t, err)
	_, err = f.CreateDependencyReader()
	require.NoError(t, err)
	err = f.Close()
	require.NoError(t, err)
	err = f.Purge(context.Background())
	require.NoError(t, err)
}

func TestTraceWriterErr(t *testing.T) {
	cfg := escfg.Configuration{
		Tags: escfg.TagsAsFields{
			File: "fixtures/file-does-not-exist.txt",
		},
	}
	coreFactory := getTestingFactoryBase(t, &cfg)
	f := &Factory{coreFactory: coreFactory, config: &cfg}
	t.Cleanup(func() {
		require.NoError(t, f.Close())
	})
	r, err := f.CreateTraceWriter()
	require.ErrorContains(t, err, "open fixtures/file-does-not-exist.txt: no such file or directory")
	assert.Nil(t, r)
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
	factory, err := NewFactory(context.Background(), cfg, telemetry.NoopSettings())
	require.NoError(t, err)
	defer factory.Close()
}

func TestESStorageFactoryErr(t *testing.T) {
	f, err := NewFactory(context.Background(), escfg.Configuration{}, telemetry.NoopSettings())
	require.ErrorContains(t, err, "failed to create Elasticsearch client: no servers specified")
	require.Nil(t, f)
}

func getTestingFactoryBase(t *testing.T, cfg *escfg.Configuration) *elasticsearch.FactoryBase {
	f := &elasticsearch.FactoryBase{}
	err := elasticsearch.SetFactoryForTest(f, zaptest.NewLogger(t), metrics.NullFactory, cfg)
	require.NoError(t, err)
	return f
}
