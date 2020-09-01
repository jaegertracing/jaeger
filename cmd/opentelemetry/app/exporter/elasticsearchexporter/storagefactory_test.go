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
	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/pkg/es/config"
	"github.com/jaegertracing/jaeger/plugin/storage/es/spanstore/dbmodel"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/plugin/storage/es"
)

func TestFactoryCreateSpanWriter(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		opts := es.NewOptionsFromConfig(config.Configuration{Version: 6, Servers: []string{"foo:9200"}}, config.Configuration{})
		factory := NewStorageFactory(opts, zap.NewNop())
		assert.NoError(t, factory.Initialize(nil, nil))
		writer, err := factory.CreateSpanWriter()
		require.NoError(t, err)
		assert.NotNil(t, writer)
	})
	t.Run("error_es_client", func(t *testing.T) {
		opts := es.NewOptionsFromConfig(config.Configuration{}, config.Configuration{})
		factory := NewStorageFactory(opts, zap.NewNop())
		writer, err := factory.CreateSpanWriter()
		require.Error(t, err)
		assert.Nil(t, writer)
	})
}

func TestFactoryCreateSpanReader(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		opts := es.NewOptionsFromConfig(config.Configuration{Version: 6, Servers: []string{"foo:9200"}}, config.Configuration{})
		factory := NewStorageFactory(opts, zap.NewNop())
		reader, err := factory.CreateSpanReader()
		require.NoError(t, err)
		assert.NotNil(t, reader)
	})
	t.Run("error_es_client", func(t *testing.T) {
		opts := es.NewOptionsFromConfig(config.Configuration{}, config.Configuration{})
		factory := NewStorageFactory(opts, zap.NewNop())
		reader, err := factory.CreateSpanReader()
		require.Error(t, err)
		assert.Nil(t, reader)
	})
}

func TestFactoryCreateDependencyReader(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		opts := es.NewOptionsFromConfig(config.Configuration{Version: 6, Servers: []string{"foo:9200"}}, config.Configuration{})
		factory := NewStorageFactory(opts, zap.NewNop())
		reader, err := factory.CreateDependencyReader()
		require.NoError(t, err)
		assert.NotNil(t, reader)
	})
	t.Run("error_es_client", func(t *testing.T) {
		opts := es.NewOptionsFromConfig(config.Configuration{}, config.Configuration{})
		factory := NewStorageFactory(opts, zap.NewNop())
		reader, err := factory.CreateDependencyReader()
		require.Error(t, err)
		assert.Nil(t, reader)
	})
}

func TestFactoryCreateArchiveSpanReader(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		opts := es.NewOptionsFromConfig(config.Configuration{}, config.Configuration{Version: 6, Servers: []string{"foo:9200"}})
		factory := NewStorageFactory(opts, zap.NewNop())
		reader, err := factory.CreateArchiveSpanReader()
		require.NoError(t, err)
		assert.NotNil(t, reader)
	})
	t.Run("error_es_client", func(t *testing.T) {
		opts := es.NewOptionsFromConfig(config.Configuration{}, config.Configuration{})
		factory := NewStorageFactory(opts, zap.NewNop())
		reader, err := factory.CreateArchiveSpanReader()
		require.Error(t, err)
		assert.Nil(t, reader)
	})
}

func TestFactoryCreateArchiveSpanWriter(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		opts := es.NewOptionsFromConfig(config.Configuration{}, config.Configuration{Version: 6, Servers: []string{"foo:9200"}})
		factory := NewStorageFactory(opts, zap.NewNop())
		writer, err := factory.CreateArchiveSpanWriter()
		require.NoError(t, err)
		assert.NotNil(t, writer)
	})
	t.Run("error_es_client", func(t *testing.T) {
		opts := es.NewOptionsFromConfig(config.Configuration{}, config.Configuration{})
		factory := NewStorageFactory(opts, zap.NewNop())
		writer, err := factory.CreateArchiveSpanWriter()
		require.Error(t, err)
		assert.Nil(t, writer)
	})
}

func TestSingleSpanWriter(t *testing.T) {
   converter := dbmodel.NewFromDomain(false, []string{}, "@")
    mw := &mockWriter{}
	w := singleSpanWriter{
		writer: mw,
		converter: converter,
	}
	s := &model.Span{
		OperationName: "foo",
		Process: model.NewProcess("service", []model.KeyValue{}),
	}
	err := w.WriteSpan(s)
	require.NoError(t, err)
	dbSpan := converter.FromDomainEmbedProcess(s)
	assert.Equal(t, []*dbmodel.Span{dbSpan}, mw.spans)
}

type mockWriter struct {
	spans []*dbmodel.Span
}

var _ batchSpanWriter = (*mockWriter)(nil)

func (m *mockWriter) writeSpans(_ context.Context, spans []*dbmodel.Span) (int, error) {
	m.spans = append(spans)
	return 0, nil
}
