// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package cassandra

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"

	"github.com/jaegertracing/jaeger/internal/metrics"
	"github.com/jaegertracing/jaeger/internal/storage/cassandra/config"
	"github.com/jaegertracing/jaeger/internal/storage/cassandra/mocks"
	"github.com/jaegertracing/jaeger/internal/storage/v1/cassandra"
	"github.com/jaegertracing/jaeger/internal/telemetry"
)

func TestNewFactoryWithConfig(t *testing.T) {
	t.Run("valid configuration", func(t *testing.T) {
		opts := &cassandra.Options{
			Configuration: config.DefaultConfiguration(),
		}
		f := cassandra.NewFactory()
		b := &withConfigBuilder{
			f:              f,
			opts:           opts,
			metricsFactory: metrics.NullFactory,
			logger:         zap.NewNop(),
			initializer:    func(_ metrics.Factory, _ *zap.Logger, _ trace.TracerProvider) error { return nil },
		}
		_, err := b.build()
		require.NoError(t, err)
	})
	t.Run("connection error", func(t *testing.T) {
		expErr := errors.New("made-up error")
		opts := &cassandra.Options{
			Configuration: config.DefaultConfiguration(),
		}
		f := cassandra.NewFactory()
		b := &withConfigBuilder{
			f:              f,
			opts:           opts,
			metricsFactory: metrics.NullFactory,
			logger:         zap.NewNop(),
			initializer:    func(_ metrics.Factory, _ *zap.Logger, _ trace.TracerProvider) error { return expErr },
		}
		_, err := b.build()
		require.ErrorIs(t, err, expErr)
	})
	t.Run("invalid configuration", func(t *testing.T) {
		cfg := cassandra.Options{}
		_, err := NewFactory(cfg, telemetry.NoopSettings())
		require.ErrorContains(t, err, "Servers: non zero value required")
	})
}

func TestNewFactory(t *testing.T) {
	v1Factory := cassandra.NewFactory()
	v1Factory.Options = cassandra.NewOptions()
	var (
		session = &mocks.Session{}
		query   = &mocks.Query{}
	)
	session.On("Query", mock.AnythingOfType("string"), mock.Anything).Return(query)
	session.On("Close").Return()
	query.On("Exec").Return(nil)
	cassandra.MockSession(v1Factory, session, nil)
	require.NoError(t, v1Factory.Initialize(metrics.NullFactory, zap.NewNop(), noop.NewTracerProvider()))
	f := createFactory(t, v1Factory)
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
	require.NoError(t, v1Factory.Initialize(metrics.NullFactory, zap.NewNop(), noop.NewTracerProvider()))
	f := createFactory(t, v1Factory)
	r, err := f.CreateTraceReader()
	require.ErrorContains(t, err, "neither table operation_names_v2 nor operation_names exist")
	require.Nil(t, r)
}

func TestCreateTraceWriterErr(t *testing.T) {
	tests := []struct {
		name        string
		factoryFunc func() *Factory
		expectedErr string
	}{
		{
			name: "error from writerOptions",
			factoryFunc: func() *Factory {
				v1Factory := cassandra.NewFactory()
				v1Factory.Options = &cassandra.Options{
					Configuration: config.DefaultConfiguration(),
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
				require.NoError(t, v1Factory.Initialize(metrics.NullFactory, zap.NewNop(), noop.NewTracerProvider()))
				f := createFactory(t, v1Factory)
				return f
			},
			expectedErr: "only one of TagIndexBlacklist and TagIndexWhitelist can be specified",
		},
		{
			name: "error from NewTraceWriter",
			factoryFunc: func() *Factory {
				v1Factory := cassandra.NewFactory()
				tableCheckStmt := "SELECT * from %s limit 1"
				session := &mocks.Session{}
				query := &mocks.Query{}
				query.On("Exec").Return(errors.New("some error"))
				session.On("Query",
					fmt.Sprintf(tableCheckStmt, "operation_names"),
					mock.Anything).Return(query)
				session.On("Query",
					fmt.Sprintf(tableCheckStmt, "operation_names_v2"),
					mock.Anything).Return(query)
				cassandra.MockSession(v1Factory, session, nil)
				require.NoError(t, v1Factory.Initialize(metrics.NullFactory, zap.NewNop(), noop.NewTracerProvider()))
				f := createFactory(t, v1Factory)
				return f
			},
			expectedErr: "neither table operation_names_v2 nor operation_names exist",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := test.factoryFunc().CreateTraceWriter()
			require.ErrorContains(t, err, test.expectedErr)
		})
	}
}

func Test_writerOptionsEdgeCases(t *testing.T) {
	tests := []struct {
		name    string
		options *cassandra.Options
	}{
		{
			name: "blacklist filter",
			options: &cassandra.Options{
				Index: cassandra.IndexConfig{
					Logs:         true,
					Tags:         false,
					ProcessTags:  true,
					TagBlackList: "tag1,tag2",
				},
			},
		},
		{
			name: "whitelist filter",
			options: &cassandra.Options{
				Index: cassandra.IndexConfig{
					Logs:         true,
					Tags:         true,
					ProcessTags:  true,
					TagWhiteList: "tag1,tag2",
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			opts, err := writerOptions(test.options)
			require.NoError(t, err)
			require.Len(t, opts, 1)
		})
	}
}

func createFactory(t *testing.T, v1Factory *cassandra.Factory) *Factory {
	return &Factory{
		v1Factory:      v1Factory,
		metricsFactory: metrics.NullFactory,
		logger:         zaptest.NewLogger(t),
		tracer:         noop.NewTracerProvider(),
	}
}
