// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package elasticsearch

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"

	"github.com/jaegertracing/jaeger/internal/metrics"
	escfg "github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
	"github.com/jaegertracing/jaeger/internal/storage/v1/elasticsearch"
)

func TestNewFactory(t *testing.T) {
	coreFactory := elasticsearch.GetTestingFactoryBase(t)
	f := NewFactory()
	f.coreFactory = coreFactory
	require.NoError(t, f.coreFactory.Initialize(metrics.NullFactory, zaptest.NewLogger(t)))
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
	coreFactory := elasticsearch.GetTestingFactoryBase(t)
	f := NewFactory()
	f.coreFactory = coreFactory
	f.coreFactory.SetConfig(&escfg.Configuration{
		UseILM:              true,
		UseReadWriteAliases: false,
	})
	require.NoError(t, f.coreFactory.Initialize(metrics.NullFactory, zaptest.NewLogger(t)))
	_, err := f.CreateTraceReader()
	require.ErrorContains(t, err, "--es.use-ilm must always be used in conjunction with --es.use-aliases to ensure ES writers and readers refer to the single index mapping")
}

func TestTraceWriterErr(t *testing.T) {
	coreFactory := elasticsearch.GetTestingFactoryBase(t)
	f := NewFactory()
	f.coreFactory = coreFactory
	f.coreFactory.SetConfig(&escfg.Configuration{
		UseILM:              true,
		UseReadWriteAliases: false,
	})
	require.NoError(t, f.coreFactory.Initialize(metrics.NullFactory, zaptest.NewLogger(t)))
	_, err := f.CreateTraceWriter()
	require.ErrorContains(t, err, "--es.use-ilm must always be used in conjunction with --es.use-aliases to ensure ES writers and readers refer to the single index mapping")
}

func TestCreateTemplatesErr(t *testing.T) {
	coreFactory := elasticsearch.GetTestingFactoryBaseWithCreateTemplateError(t, errors.New("template error"))
	f := NewFactory()
	f.coreFactory = coreFactory
	f.coreFactory.SetConfig(&escfg.Configuration{
		UseILM:               false,
		CreateIndexTemplates: true,
	})
	require.NoError(t, f.coreFactory.Initialize(metrics.NullFactory, zaptest.NewLogger(t)))
	_, err := f.CreateTraceWriter()
	require.ErrorContains(t, err, "template error")
}
