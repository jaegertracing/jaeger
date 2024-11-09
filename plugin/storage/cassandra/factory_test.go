// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package cassandra

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/pkg/cassandra"
	cassandraCfg "github.com/jaegertracing/jaeger/pkg/cassandra/config"
	"github.com/jaegertracing/jaeger/pkg/cassandra/mocks"
	"github.com/jaegertracing/jaeger/pkg/config"
	"github.com/jaegertracing/jaeger/pkg/metrics"
	"github.com/jaegertracing/jaeger/pkg/testutils"
)

type mockSessionBuilder struct {
	session *mocks.Session
	err     error
}

func newMockSessionBuilder(session *mocks.Session, err error) *mockSessionBuilder {
	return &mockSessionBuilder{
		session: session,
		err:     err,
	}
}

func (m *mockSessionBuilder) NewSession() (cassandra.Session, error) {
	return m.session, m.err
}

func TestCassandraFactory(t *testing.T) {
	logger, _ := testutils.NewLogger()
	f := NewFactory(false)
	v, _ := config.Viperize(f.AddFlags)
	f.InitFromViper(v, zap.NewNop())

	// after InitFromViper, f.primaryConfig points to a real session builder that will fail in unit tests,
	// so we override it with a mock.
	f.config = newMockSessionBuilder(nil, errors.New("made-up error"))
	require.EqualError(t, f.Initialize(metrics.NullFactory, zap.NewNop()), "made-up error")

	var (
		session = &mocks.Session{}
		query   = &mocks.Query{}
	)
	session.On("Query", mock.AnythingOfType("string"), mock.Anything).Return(query)
	session.On("Close").Return()
	query.On("Exec").Return(nil)
	f.config = newMockSessionBuilder(nil, errors.New("made-up error"))
	require.EqualError(t, f.Initialize(metrics.NullFactory, zap.NewNop()), "made-up error")

	f.config = newMockSessionBuilder(session, nil)
	require.NoError(t, f.Initialize(metrics.NullFactory, logger))

	_, err := f.CreateSpanReader()
	require.NoError(t, err)

	_, err = f.CreateSpanWriter()
	require.NoError(t, err)

	_, err = f.CreateDependencyReader()
	require.NoError(t, err)

	_, err = f.CreateLock()
	require.NoError(t, err)

	_, err = f.CreateSamplingStore(0)
	require.NoError(t, err)

	require.NoError(t, f.Close())
}

func TestExclusiveWhitelistBlacklist(t *testing.T) {
	f := NewFactory(false)
	v, command := config.Viperize(f.AddFlags)
	command.ParseFlags([]string{
		"--cassandra.index.tag-whitelist=a,b,c",
		"--cassandra.index.tag-blacklist=a,b,c",
	})
	f.InitFromViper(v, zap.NewNop())

	// after InitFromViper, f.primaryConfig points to a real session builder that will fail in unit tests,
	// so we override it with a mock.
	f.config = newMockSessionBuilder(nil, errors.New("made-up error"))
	require.EqualError(t, f.Initialize(metrics.NullFactory, zap.NewNop()), "made-up error")

	var (
		session = &mocks.Session{}
		query   = &mocks.Query{}
	)
	session.On("Query", mock.AnythingOfType("string"), mock.Anything).Return(query)
	query.On("Exec").Return(nil)
	f.config = newMockSessionBuilder(session, nil)

	_, err := f.CreateSpanWriter()
	require.EqualError(t, err, "only one of TagIndexBlacklist and TagIndexWhitelist can be specified")
}

func TestWriterOptions(t *testing.T) {
	opts := NewOptions("cassandra")
	v, command := config.Viperize(opts.AddFlags)
	command.ParseFlags([]string{"--cassandra.index.tag-whitelist=a,b,c"})
	opts.InitFromViper(v)

	options, _ := writerOptions(opts)
	assert.Len(t, options, 1)

	opts = NewOptions("cassandra")
	v, command = config.Viperize(opts.AddFlags)
	command.ParseFlags([]string{"--cassandra.index.tag-blacklist=a,b,c"})
	opts.InitFromViper(v)

	options, _ = writerOptions(opts)
	assert.Len(t, options, 1)

	opts = NewOptions("cassandra")
	v, command = config.Viperize(opts.AddFlags)
	command.ParseFlags([]string{"--cassandra.index.tags=false"})
	opts.InitFromViper(v)

	options, _ = writerOptions(opts)
	assert.Len(t, options, 1)

	opts = NewOptions("cassandra")
	v, command = config.Viperize(opts.AddFlags)
	command.ParseFlags([]string{"--cassandra.index.tags=false", "--cassandra.index.tag-blacklist=a,b,c"})
	opts.InitFromViper(v)

	options, _ = writerOptions(opts)
	assert.Len(t, options, 1)

	opts = NewOptions("cassandra")
	v, command = config.Viperize(opts.AddFlags)
	command.ParseFlags([]string{""})
	opts.InitFromViper(v)

	options, _ = writerOptions(opts)
	assert.Empty(t, options)
}

func TestConfigureFromOptions(t *testing.T) {
	f := NewFactory(false)
	o := NewOptions("foo")
	f.configureFromOptions(o)
	assert.Equal(t, o, f.Options)
	assert.Equal(t, o.GetPrimary(), f.config)
}

func TestNewFactoryWithConfig(t *testing.T) {
	t.Run("valid configuration", func(t *testing.T) {
		opts := &Options{
			Primary: NamespaceConfig{
				Configuration: cassandraCfg.Configuration{
					Connection: cassandraCfg.Connection{
						Servers: []string{"localhost:9200"},
					},
				},
			},
		}
		f := NewFactory(false)
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
		opts := &Options{
			Primary: NamespaceConfig{
				Configuration: cassandraCfg.Configuration{
					Connection: cassandraCfg.Connection{
						Servers: []string{"localhost:9200"},
					},
				},
			},
		}
		f := NewFactory(false)
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
		cfg := Options{}
		_, err := NewFactoryWithConfig(cfg, metrics.NullFactory, zap.NewNop())
		require.ErrorContains(t, err, "Servers: non zero value required")
	})
}

func TestFactory_Purge(t *testing.T) {
	f := NewFactory(false)
	var (
		session = &mocks.Session{}
		query   = &mocks.Query{}
	)
	session.On("Query", mock.AnythingOfType("string"), mock.Anything).Return(query)
	query.On("Exec").Return(nil)
	f.session = session

	err := f.Purge(context.Background())
	require.NoError(t, err)

	session.AssertCalled(t, "Query", mock.AnythingOfType("string"), mock.Anything)
	query.AssertCalled(t, "Exec")
}
