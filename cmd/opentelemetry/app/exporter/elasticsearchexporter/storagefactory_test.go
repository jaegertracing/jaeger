// Copyright (c) 2020 The Jaeger Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package elasticsearchexporter

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/opentelemetry/app/exporter/elasticsearchexporter/esmodeltranslator"
	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/pkg/es/config"
	"github.com/jaegertracing/jaeger/plugin/storage/es"
	"github.com/jaegertracing/jaeger/plugin/storage/es/spanstore/dbmodel"
)

const testExporterName = "test"

func TestFactoryCreateSpanWriter(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		opts := es.NewOptionsFromConfig(config.Configuration{Version: 6, Servers: []string{"foo:9200"}}, config.Configuration{})
		factory := NewStorageFactory(opts, zap.NewNop(), testExporterName)
		assert.NoError(t, factory.Initialize(nil, nil))
		writer, err := factory.CreateSpanWriter()
		require.NoError(t, err)
		assert.NotNil(t, writer)
	})
	t.Run("error_es_client", func(t *testing.T) {
		opts := es.NewOptionsFromConfig(config.Configuration{}, config.Configuration{})
		factory := NewStorageFactory(opts, zap.NewNop(), testExporterName)
		writer, err := factory.CreateSpanWriter()
		require.Error(t, err)
		assert.Nil(t, writer)
	})
}

func TestFactoryCreateSpanReader(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		opts := es.NewOptionsFromConfig(config.Configuration{Version: 6, Servers: []string{"foo:9200"}}, config.Configuration{})
		factory := NewStorageFactory(opts, zap.NewNop(), testExporterName)
		reader, err := factory.CreateSpanReader()
		require.NoError(t, err)
		assert.NotNil(t, reader)
	})
	t.Run("error_es_client", func(t *testing.T) {
		opts := es.NewOptionsFromConfig(config.Configuration{}, config.Configuration{})
		factory := NewStorageFactory(opts, zap.NewNop(), testExporterName)
		reader, err := factory.CreateSpanReader()
		require.Error(t, err)
		assert.Nil(t, reader)
	})
}

func TestFactoryCreateDependencyReader(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		opts := es.NewOptionsFromConfig(config.Configuration{Version: 6, Servers: []string{"foo:9200"}}, config.Configuration{})
		factory := NewStorageFactory(opts, zap.NewNop(), testExporterName)
		reader, err := factory.CreateDependencyReader()
		require.NoError(t, err)
		assert.NotNil(t, reader)
	})
	t.Run("error_es_client", func(t *testing.T) {
		opts := es.NewOptionsFromConfig(config.Configuration{}, config.Configuration{})
		factory := NewStorageFactory(opts, zap.NewNop(), testExporterName)
		reader, err := factory.CreateDependencyReader()
		require.Error(t, err)
		assert.Nil(t, reader)
	})
}

func TestFactoryCreateArchiveSpanReader(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		opts := es.NewOptionsFromConfig(config.Configuration{}, config.Configuration{Version: 6, Servers: []string{"foo:9200"}})
		factory := NewStorageFactory(opts, zap.NewNop(), testExporterName)
		reader, err := factory.CreateArchiveSpanReader()
		require.NoError(t, err)
		assert.NotNil(t, reader)
	})
	t.Run("error_es_client", func(t *testing.T) {
		opts := es.NewOptionsFromConfig(config.Configuration{}, config.Configuration{})
		factory := NewStorageFactory(opts, zap.NewNop(), testExporterName)
		reader, err := factory.CreateArchiveSpanReader()
		require.Error(t, err)
		assert.Nil(t, reader)
	})
}

func TestFactoryCreateArchiveSpanWriter(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		opts := es.NewOptionsFromConfig(config.Configuration{}, config.Configuration{Version: 6, Servers: []string{"foo:9200"}})
		factory := NewStorageFactory(opts, zap.NewNop(), testExporterName)
		writer, err := factory.CreateArchiveSpanWriter()
		require.NoError(t, err)
		assert.NotNil(t, writer)
	})
	t.Run("error_es_client", func(t *testing.T) {
		opts := es.NewOptionsFromConfig(config.Configuration{}, config.Configuration{})
		factory := NewStorageFactory(opts, zap.NewNop(), testExporterName)
		writer, err := factory.CreateArchiveSpanWriter()
		require.Error(t, err)
		assert.Nil(t, writer)
	})
}

func TestSingleSpanWriter(t *testing.T) {
	converter := dbmodel.NewFromDomain(false, []string{}, "@")
	mw := &mockWriter{}
	w := singleSpanWriter{
		writer:    mw,
		converter: converter,
	}
	s := &model.Span{
		OperationName: "foo",
		Process:       model.NewProcess("service", []model.KeyValue{}),
	}
	err := w.WriteSpan(context.Background(), s)
	require.NoError(t, err)
	dbSpan := converter.FromDomainEmbedProcess(s)
	assert.Equal(t, []*dbmodel.Span{dbSpan}, mw.spans)
}

type mockWriter struct {
	spans []*dbmodel.Span
}

var _ batchSpanWriter = (*mockWriter)(nil)

func (m *mockWriter) writeSpans(ctx context.Context, spansData []esmodeltranslator.ConvertedData) (int, error) {
	for _, c := range spansData {
		m.spans = append(m.spans, c.DBSpan)
	}
	return 0, nil
}
