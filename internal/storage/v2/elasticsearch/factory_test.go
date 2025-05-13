// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package elasticsearch

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

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
	coreFactory := getTestingFactoryBase(t, &escfg.Configuration{})
	f := &Factory{coreFactory: coreFactory}
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

func TestTraceReaderErr(t *testing.T) {
	coreFactory := getTestingFactoryBase(t, &escfg.Configuration{
		UseILM:              true,
		UseReadWriteAliases: false,
	})
	f := &Factory{coreFactory: coreFactory}
	_, err := f.CreateTraceReader()
	require.ErrorContains(t, err, "--es.use-ilm must always be used in conjunction with --es.use-aliases to ensure ES writers and readers refer to the single index mapping")
}

func TestTraceWriterErr(t *testing.T) {
	coreFactory := getTestingFactoryBase(t, &escfg.Configuration{
		UseILM:              true,
		UseReadWriteAliases: false,
	})
	f := &Factory{coreFactory: coreFactory}
	_, err := f.CreateTraceWriter()
	require.ErrorContains(t, err, "--es.use-ilm must always be used in conjunction with --es.use-aliases to ensure ES writers and readers refer to the single index mapping")
}

func TestCreateTemplatesErr(t *testing.T) {
	cfg := &escfg.Configuration{
		UseILM:               false,
		CreateIndexTemplates: true,
	}
	coreFactory := getTestingFactoryBaseWithCreateTemplateError(t, cfg, errors.New("template error"))
	f := &Factory{coreFactory: coreFactory}
	_, err := f.CreateTraceWriter()
	require.ErrorContains(t, err, "template error")
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
	factory, err := NewFactoryWithConfig(cfg, telemetry.NoopSettings())
	require.NoError(t, err)
	defer factory.Close()
}

func TestESStorageFactoryWithConfigError(t *testing.T) {
	f, err := NewFactoryWithConfig(escfg.Configuration{}, telemetry.NoopSettings())
	require.ErrorContains(t, err, "Servers: non zero value required")
	require.Nil(t, f)
}

func TestTraceWriterMappingBuilderErr(t *testing.T) {
	coreFactory := &elasticsearch.FactoryBase{}
	err := elasticsearch.SetFactoryForTestWithMappingErr(coreFactory, zaptest.NewLogger(t), metrics.NullFactory, &escfg.Configuration{CreateIndexTemplates: true}, errors.New("template-error"))
	require.NoError(t, err)
	f := &Factory{coreFactory: coreFactory}
	_, err = f.CreateTraceWriter()
	require.ErrorContains(t, err, "template-error")
}

func getTestingFactoryBase(t *testing.T, cfg *escfg.Configuration) *elasticsearch.FactoryBase {
	f := &elasticsearch.FactoryBase{}
	err := elasticsearch.SetFactoryForTest(f, zaptest.NewLogger(t), metrics.NullFactory, cfg)
	require.NoError(t, err)
	return f
}

func getTestingFactoryBaseWithCreateTemplateError(t *testing.T, cfg *escfg.Configuration, templateErr error) *elasticsearch.FactoryBase {
	f := &elasticsearch.FactoryBase{}
	err := elasticsearch.SetFactoryForTestWithCreateTemplateErr(f, zaptest.NewLogger(t), metrics.NullFactory, cfg, templateErr)
	require.NoError(t, err)
	return f
}
