// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package cassandra

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/metrics"
	"github.com/jaegertracing/jaeger/internal/storage/cassandra/config"
	"github.com/jaegertracing/jaeger/internal/storage/cassandra/mocks"
	"github.com/jaegertracing/jaeger/internal/storage/v1/cassandra"
)

func TestNewFactoryWithConfig(t *testing.T) {
	t.Run("valid configuration", func(t *testing.T) {
		opts := &cassandra.Options{
			NamespaceConfig: cassandra.NamespaceConfig{
				Configuration: config.DefaultConfiguration(),
			},
		}
		f := cassandra.NewFactory()
		b := &withConfigBuilder{
			f:              f,
			opts:           opts,
			metricsFactory: metrics.NullFactory,
			logger:         zap.NewNop(),
			initializer:    func(_ metrics.Factory, _ *zap.Logger) error { return nil },
		}
		_, err := b.build()
		require.NoError(t, err)
	})
	t.Run("connection error", func(t *testing.T) {
		expErr := errors.New("made-up error")
		opts := &cassandra.Options{
			NamespaceConfig: cassandra.NamespaceConfig{
				Configuration: config.DefaultConfiguration(),
			},
		}
		f := cassandra.NewFactory()
		b := &withConfigBuilder{
			f:              f,
			opts:           opts,
			metricsFactory: metrics.NullFactory,
			logger:         zap.NewNop(),
			initializer:    func(_ metrics.Factory, _ *zap.Logger) error { return expErr },
		}
		_, err := b.build()
		require.ErrorIs(t, err, expErr)
	})
	t.Run("invalid configuration", func(t *testing.T) {
		cfg := cassandra.Options{}
		_, err := NewFactory(cfg, metrics.NullFactory, zap.NewNop())
		require.ErrorContains(t, err, "Servers: non zero value required")
	})
}

func TestNewFactory(t *testing.T) {
	v1Factory := cassandra.NewFactory()
	v1Factory.Options = cassandra.NewOptions("primary")
	var (
		session = &mocks.Session{}
		query   = &mocks.Query{}
	)
	session.On("Query", mock.AnythingOfType("string"), mock.Anything).Return(query)
	session.On("Close").Return()
	query.On("Exec").Return(nil)
	cassandra.MockSession(v1Factory, session, nil)
	require.NoError(t, v1Factory.Initialize(metrics.NullFactory, zap.NewNop()))
	f := &Factory{v1Factory: v1Factory}
	_, err := f.CreateTraceWriter()
	require.NoError(t, err)

	_, err = f.CreateTraceReader()
	require.NoError(t, err)

	_, err = f.CreateDependencyReader()
	require.NoError(t, err)

	_, err = f.CreateLock()
	require.NoError(t, err)

	_, err = f.CreateSamplingStore(0)
	require.NoError(t, err)

	require.NoError(t, f.Close())
}

func TestCreateTraceReaderError(t *testing.T) {
	session := &mocks.Session{}
	query := &mocks.Query{}
	session.On("Query",
		mock.AnythingOfType("string"),
		mock.Anything).Return(query)
	session.On("Query",
		mock.AnythingOfType("string"),
		mock.Anything).Return(query)
	query.On("Exec").Return(errors.New("table does not exist"))
	v1Factory := cassandra.NewFactory()
	cassandra.MockSession(v1Factory, session, nil)
	require.NoError(t, v1Factory.Initialize(metrics.NullFactory, zap.NewNop()))
	f := &Factory{v1Factory: v1Factory}
	r, err := f.CreateTraceReader()
	require.ErrorContains(t, err, "neither table operation_names_v2 nor operation_names exist")
	require.Nil(t, r)
}

func TestCreateTraceWriterErr(t *testing.T) {
	v1Factory := cassandra.NewFactory()
	v1Factory.Options = &cassandra.Options{
		NamespaceConfig: cassandra.NamespaceConfig{
			Configuration: config.DefaultConfiguration(),
		},
		Index: cassandra.IndexConfig{
			TagBlackList: "a,b,c",
			TagWhiteList: "a,b,c",
		},
	}
	var (
		session = &mocks.Session{}
		query   = &mocks.Query{}
	)
	session.On("Query", mock.AnythingOfType("string"), mock.Anything).Return(query)
	query.On("Exec").Return(nil)
	cassandra.MockSession(v1Factory, session, nil)
	require.NoError(t, v1Factory.Initialize(metrics.NullFactory, zap.NewNop()))
	f := &Factory{v1Factory: v1Factory}
	_, err := f.CreateTraceWriter()
	require.ErrorContains(t, err, "only one of TagIndexBlacklist and TagIndexWhitelist can be specified")
}
