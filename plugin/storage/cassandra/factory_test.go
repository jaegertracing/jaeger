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
	"go.opentelemetry.io/collector/config/configtls"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/pkg/cassandra"
	"github.com/jaegertracing/jaeger/pkg/cassandra/config"
	"github.com/jaegertracing/jaeger/pkg/cassandra/mocks"
	viperize "github.com/jaegertracing/jaeger/pkg/config"
	"github.com/jaegertracing/jaeger/pkg/metrics"
	"github.com/jaegertracing/jaeger/pkg/testutils"
)

type mockSessionBuilder struct {
	index    int
	sessions []*mocks.Session
	errors   []error
}

func (m *mockSessionBuilder) add(session *mocks.Session, err error) *mockSessionBuilder {
	m.sessions = append(m.sessions, session)
	m.errors = append(m.errors, err)
	return m
}

func (m *mockSessionBuilder) build(*config.Configuration) (cassandra.Session, error) {
	if m.index >= len(m.sessions) {
		return nil, errors.New("no more sessions")
	}
	session := m.sessions[m.index]
	err := m.errors[m.index]
	m.index++
	return session, err
}

func TestCassandraFactory(t *testing.T) {
	logger, _ := testutils.NewLogger()
	f := NewFactory()
	v, command := viperize.Viperize(f.AddFlags)
	command.ParseFlags([]string{"--cassandra-archive.enabled=true"})
	f.InitFromViper(v, zap.NewNop())

	f.sessionBuilderFn = new(mockSessionBuilder).add(nil, errors.New("made-up primary error")).build
	require.EqualError(t, f.Initialize(metrics.NullFactory, zap.NewNop()), "made-up primary error")

	var (
		session = &mocks.Session{}
		query   = &mocks.Query{}
	)
	session.On("Query", mock.AnythingOfType("string"), mock.Anything).Return(query)
	session.On("Close").Return()
	query.On("Exec").Return(nil)

	f.sessionBuilderFn = new(mockSessionBuilder).add(session, nil).build
	require.NoError(t, f.Initialize(metrics.NullFactory, logger))

	_, err := f.CreateSpanReader()
	require.NoError(t, err)

	_, err = f.CreateSpanWriter()
	require.NoError(t, err)

	_, err = f.CreateDependencyReader()
	require.NoError(t, err)

	f.sessionBuilderFn = new(mockSessionBuilder).add(session, nil).add(session, nil).build
	require.NoError(t, f.Initialize(metrics.NullFactory, zap.NewNop()))

	_, err = f.CreateLock()
	require.NoError(t, err)

	_, err = f.CreateSamplingStore(0)
	require.NoError(t, err)

	require.NoError(t, f.Close())
}

func TestCreateSpanReaderError(t *testing.T) {
	session := &mocks.Session{}
	query := &mocks.Query{}
	session.On("Query",
		mock.AnythingOfType("string"),
		mock.Anything).Return(query)
	session.On("Query",
		mock.AnythingOfType("string"),
		mock.Anything).Return(query)
	query.On("Exec").Return(errors.New("table does not exist"))
	f := NewFactory()
	f.sessionBuilderFn = new(mockSessionBuilder).add(session, nil).add(session, nil).build
	require.NoError(t, f.Initialize(metrics.NullFactory, zap.NewNop()))
	r, err := f.CreateSpanReader()
	require.Error(t, err)
	require.Nil(t, r)
}

func TestExclusiveWhitelistBlacklist(t *testing.T) {
	f := NewFactory()
	v, command := viperize.Viperize(f.AddFlags)
	command.ParseFlags([]string{
		"--cassandra.index.tag-whitelist=a,b,c",
		"--cassandra.index.tag-blacklist=a,b,c",
	})
	f.InitFromViper(v, zap.NewNop())

	var (
		session = &mocks.Session{}
		query   = &mocks.Query{}
	)
	session.On("Query", mock.AnythingOfType("string"), mock.Anything).Return(query)
	query.On("Exec").Return(nil)
	f.sessionBuilderFn = new(mockSessionBuilder).add(session, nil).build

	_, err := f.CreateSpanWriter()
	require.EqualError(t, err, "only one of TagIndexBlacklist and TagIndexWhitelist can be specified")

	f.sessionBuilderFn = new(mockSessionBuilder).add(session, nil).add(session, nil).build
	require.NoError(t, f.Initialize(metrics.NullFactory, zap.NewNop()))
}

func TestWriterOptions(t *testing.T) {
	opts := NewOptions("cassandra")
	v, command := viperize.Viperize(opts.AddFlags)
	command.ParseFlags([]string{"--cassandra.index.tag-whitelist=a,b,c"})
	opts.InitFromViper(v)

	options, _ := writerOptions(opts)
	assert.Len(t, options, 1)

	opts = NewOptions("cassandra")
	v, command = viperize.Viperize(opts.AddFlags)
	command.ParseFlags([]string{"--cassandra.index.tag-blacklist=a,b,c"})
	opts.InitFromViper(v)

	options, _ = writerOptions(opts)
	assert.Len(t, options, 1)

	opts = NewOptions("cassandra")
	v, command = viperize.Viperize(opts.AddFlags)
	command.ParseFlags([]string{"--cassandra.index.tags=false"})
	opts.InitFromViper(v)

	options, _ = writerOptions(opts)
	assert.Len(t, options, 1)

	opts = NewOptions("cassandra")
	v, command = viperize.Viperize(opts.AddFlags)
	command.ParseFlags([]string{"--cassandra.index.tags=false", "--cassandra.index.tag-blacklist=a,b,c"})
	opts.InitFromViper(v)

	options, _ = writerOptions(opts)
	assert.Len(t, options, 1)

	opts = NewOptions("cassandra")
	v, command = viperize.Viperize(opts.AddFlags)
	command.ParseFlags([]string{""})
	opts.InitFromViper(v)

	options, _ = writerOptions(opts)
	assert.Empty(t, options)
}

func TestConfigureFromOptions(t *testing.T) {
	f := NewFactory()
	o := NewOptions("foo")
	f.configureFromOptions(o)
	assert.Equal(t, o, f.Options)
	assert.Equal(t, o.GetConfig(), f.config)
}

func TestNewFactoryWithConfig(t *testing.T) {
	t.Run("valid configuration", func(t *testing.T) {
		opts := &Options{
			NamespaceConfig: NamespaceConfig{
				Configuration: config.DefaultConfiguration(),
			},
		}
		f := NewFactory()
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
			NamespaceConfig: NamespaceConfig{
				Configuration: config.DefaultConfiguration(),
			},
		}
		f := NewFactory()
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
	f := NewFactory()
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

func TestNewSessionErrors(t *testing.T) {
	t.Run("NewCluster error", func(t *testing.T) {
		cfg := &config.Configuration{
			Connection: config.Connection{
				TLS: configtls.ClientConfig{
					Config: configtls.Config{
						CAFile: "foobar",
					},
				},
			},
		}
		_, err := NewSession(cfg)
		require.ErrorContains(t, err, "failed to load TLS config")
	})
	t.Run("CreateSession error", func(t *testing.T) {
		cfg := &config.Configuration{}
		_, err := NewSession(cfg)
		require.ErrorContains(t, err, "no hosts provided")
	})
	t.Run("CreateSession error with schema", func(t *testing.T) {
		cfg := &config.Configuration{
			Schema: config.Schema{
				CreateSchema: true,
			},
		}
		_, err := NewSession(cfg)
		require.ErrorContains(t, err, "no hosts provided")
	})
}
