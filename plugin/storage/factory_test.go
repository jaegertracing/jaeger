// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2018 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package storage

import (
	"errors"
	"expvar"
	"flag"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/pkg/config"
	"github.com/jaegertracing/jaeger/pkg/telemetry"
	"github.com/jaegertracing/jaeger/storage"
	"github.com/jaegertracing/jaeger/storage/dependencystore"
	depStoreMocks "github.com/jaegertracing/jaeger/storage/dependencystore/mocks"
	"github.com/jaegertracing/jaeger/storage/mocks"
	"github.com/jaegertracing/jaeger/storage/spanstore"
	spanStoreMocks "github.com/jaegertracing/jaeger/storage/spanstore/mocks"
)

func defaultCfg() FactoryConfig {
	return FactoryConfig{
		SpanWriterTypes:         []string{cassandraStorageType},
		SpanReaderType:          cassandraStorageType,
		DependenciesStorageType: cassandraStorageType,
		DownsamplingRatio:       1.0,
		DownsamplingHashSalt:    "",
	}
}

// template function
func assertNonEmptyKey[T any](t *testing.T, m map[string]T, key string) {
	require.NotEmpty(t, m)
	_, ok := m[key]
	assert.True(t, ok, "key %s is expected ", key)
}

func TestNewFactory(t *testing.T) {
	f := NewFactory(defaultCfg())
	assert.Equal(t, cassandraStorageType, f.SpanWriterTypes[0])
	assert.Equal(t, cassandraStorageType, f.SpanReaderType)
	assert.Equal(t, cassandraStorageType, f.DependenciesStorageType)
	assertNonEmptyKey(t, f.factories, cassandraStorageType)

	f = NewFactory(FactoryConfig{
		SpanWriterTypes:         []string{cassandraStorageType, kafkaStorageType, badgerStorageType},
		SpanReaderType:          elasticsearchStorageType,
		DependenciesStorageType: memoryStorageType,
	})
	assertNonEmptyKey(t, f.factories, cassandraStorageType)
	assertNonEmptyKey(t, f.factories, kafkaStorageType)
	assertNonEmptyKey(t, f.factories, badgerStorageType)
	assertNonEmptyKey(t, f.factories, elasticsearchStorageType)
	assertNonEmptyKey(t, f.factories, memoryStorageType)
	assert.Equal(t, []string{cassandraStorageType, kafkaStorageType, badgerStorageType}, f.SpanWriterTypes)
	assert.Equal(t, elasticsearchStorageType, f.SpanReaderType)
	assert.Equal(t, memoryStorageType, f.DependenciesStorageType)

	f = NewFactory(FactoryConfig{SpanWriterTypes: []string{"x"}, DependenciesStorageType: "y", SpanReaderType: "z"})
	err := f.Initialize(telemetry.NoopSettings())
	require.ErrorContains(t, err, "unknown storage type")
	require.NoError(t, f.Close())
}

func TestClose(t *testing.T) {
	storageType := "foo"
	err := errors.New("some error")
	f := Factory{
		factories: map[string]storage.Factory{
			storageType: &errorFactory{closeErr: err},
		},
		FactoryConfig: FactoryConfig{SpanWriterTypes: []string{storageType}},
	}
	require.EqualError(t, f.Close(), err.Error())
}

func TestInitialize(t *testing.T) {
	f := NewFactory(defaultCfg())
	f.telset = telemetry.NoopSettings()
	assertNonEmptyKey(t, f.factories, cassandraStorageType)

	stub := new(mocks.Factory)
	f.factories[cassandraStorageType] = stub
	stub.On("Initialize").Return(nil)
	err := f.initFactories()
	require.NoError(t, err)

	stub = new(mocks.Factory)
	f.factories[cassandraStorageType] = stub
	stub.On("Initialize").Return(errors.New("init-error"))
	err = f.initFactories()
	require.ErrorContains(t, err, "init-error")
}

func TestCreate(t *testing.T) {
	f := NewFactory(defaultCfg())

	stub := new(mocks.Factory)
	f.factories[cassandraStorageType] = stub

	spanReader := new(spanStoreMocks.Reader)
	spanWriter := new(spanStoreMocks.Writer)
	depReader := new(depStoreMocks.Reader)

	stub.On("CreateSpanReader").Return(spanReader, errors.New("span-reader-error"))
	stub.On("CreateSpanWriter").Once().Return(spanWriter, errors.New("span-writer-error"))
	stub.On("CreateDependencyReader").Return(depReader, errors.New("dep-reader-error"))

	r, err := f.CreateSpanReader()
	assert.Equal(t, spanReader, r)
	require.EqualError(t, err, "span-reader-error")

	w, err := f.CreateSpanWriter()
	assert.Nil(t, w)
	require.EqualError(t, err, "span-writer-error")

	d, err := f.CreateDependencyReader()
	assert.Equal(t, depReader, d)
	require.EqualError(t, err, "dep-reader-error")

	_, err = f.CreateArchiveSpanReader()
	require.EqualError(t, err, "archive storage not supported")

	_, err = f.CreateArchiveSpanWriter()
	require.EqualError(t, err, "archive storage not supported")

	stub.On("Initialize").Return(nil)
	stub.On("CreateSpanWriter").Return(spanWriter, nil)
	f.initFactories()
	w, err = f.CreateSpanWriter()
	require.NoError(t, err)
	assert.Equal(t, spanWriter, w)
}

func TestCreateDownsamplingWriter(t *testing.T) {
	f := NewFactory(defaultCfg())
	f.telset = telemetry.NoopSettings()

	mock := new(mocks.Factory)
	f.factories[cassandraStorageType] = mock
	spanWriter := new(spanStoreMocks.Writer)
	mock.On("Initialize").Return(nil)
	mock.On("CreateSpanWriter").Return(spanWriter, nil)

	testParams := []struct {
		ratio      float64
		writerType string
	}{
		{0.5, "*spanstore.DownsamplingWriter"},
		{1.0, "*mocks.Writer"},
	}

	for _, param := range testParams {
		t.Run(param.writerType, func(t *testing.T) {
			f.DownsamplingRatio = param.ratio
			f.initFactories()
			newWriter, err := f.CreateSpanWriter()
			require.NoError(t, err)
			// Currently directly assertEqual doesn't work since DownsamplingWriter initializes with different
			// address for hashPool. The following workaround checks writer type instead
			assert.True(t, strings.HasPrefix(reflect.TypeOf(newWriter).String(), param.writerType))
		})
	}
}

func TestCreateMulti(t *testing.T) {
	cfg := defaultCfg()
	cfg.SpanWriterTypes = append(cfg.SpanWriterTypes, elasticsearchStorageType)
	f := NewFactory(cfg)

	mock := new(mocks.Factory)
	mock2 := new(mocks.Factory)
	f.factories[cassandraStorageType] = mock
	f.factories[elasticsearchStorageType] = mock2

	spanWriter := new(spanStoreMocks.Writer)
	spanWriter2 := new(spanStoreMocks.Writer)

	mock.On("CreateSpanWriter").Once().Return(spanWriter, errors.New("span-writer-error"))

	w, err := f.CreateSpanWriter()
	assert.Nil(t, w)
	require.EqualError(t, err, "span-writer-error")

	mock.On("CreateSpanWriter").Return(spanWriter, nil)
	mock2.On("CreateSpanWriter").Return(spanWriter2, nil)

	mock.On("Initialize").Return(nil)
	mock2.On("Initialize").Return(nil)
	f.initFactories()
	w, err = f.CreateSpanWriter()
	require.NoError(t, err)
	assert.Equal(t, spanstore.NewCompositeWriter(spanWriter, spanWriter2), w)
}

func TestCreateArchive(t *testing.T) {
	f := NewFactory(defaultCfg())

	mock := &struct {
		mocks.Factory
		mocks.ArchiveFactory
	}{}
	f.factories[cassandraStorageType] = mock

	archiveSpanReader := new(spanStoreMocks.Reader)
	archiveSpanWriter := new(spanStoreMocks.Writer)

	mock.ArchiveFactory.On("CreateArchiveSpanReader").Return(archiveSpanReader, errors.New("archive-span-reader-error"))
	mock.ArchiveFactory.On("CreateArchiveSpanWriter").Return(archiveSpanWriter, errors.New("archive-span-writer-error"))

	ar, err := f.CreateArchiveSpanReader()
	assert.Equal(t, archiveSpanReader, ar)
	require.EqualError(t, err, "archive-span-reader-error")

	aw, err := f.CreateArchiveSpanWriter()
	assert.Equal(t, archiveSpanWriter, aw)
	require.EqualError(t, err, "archive-span-writer-error")
}

func TestCreateError(t *testing.T) {
	f := NewFactory(defaultCfg())
	delete(f.factories, cassandraStorageType)

	expectedErr := "no cassandra backend registered for span store"
	// scope the vars to avoid bugs in the test
	{
		r, err := f.CreateSpanReader()
		assert.Nil(t, r)
		require.EqualError(t, err, expectedErr)
	}

	{
		w, err := f.CreateSpanWriter()
		assert.Nil(t, w)
		require.EqualError(t, err, expectedErr)
	}

	{
		d, err := f.CreateDependencyReader()
		assert.Nil(t, d)
		require.EqualError(t, err, expectedErr)
	}

	{
		r, err := f.CreateArchiveSpanReader()
		assert.Nil(t, r)
		require.EqualError(t, err, expectedErr)
	}

	{
		w, err := f.CreateArchiveSpanWriter()
		assert.Nil(t, w)
		require.EqualError(t, err, expectedErr)
	}
}

func TestAllSamplingStorageTypes(t *testing.T) {
	assert.Equal(t, []string{"cassandra", "memory", "badger"}, AllSamplingStorageTypes())
}

func TestCreateSamplingStoreFactory(t *testing.T) {
	f := NewFactory(defaultCfg())

	// if not specified sampling store is chosen from available factories
	ssFactory, err := f.CreateSamplingStoreFactory()
	assert.Equal(t, f.factories[cassandraStorageType], ssFactory)
	require.NoError(t, err)

	// if not specified and there's no compatible factories then return nil
	delete(f.factories, cassandraStorageType)
	ssFactory, err = f.CreateSamplingStoreFactory()
	assert.Nil(t, ssFactory)
	require.NoError(t, err)

	// if an incompatible factory is specified return err
	cfg := defaultCfg()
	cfg.SamplingStorageType = "elasticsearch"
	f = NewFactory(cfg)
	require.NoError(t, f.buildFactories())
	ssFactory, err = f.CreateSamplingStoreFactory()
	assert.Nil(t, ssFactory)
	require.EqualError(t, err, "storage factory of type elasticsearch does not support sampling store")

	// if a compatible factory is specified then return it
	cfg.SamplingStorageType = "cassandra"
	f = NewFactory(cfg)
	require.NoError(t, f.buildFactories())
	ssFactory, err = f.CreateSamplingStoreFactory()
	assert.Equal(t, ssFactory, f.factories["cassandra"])
	require.NoError(t, err)
}

type configurable struct {
	mocks.Factory
	flagSet *flag.FlagSet
	viper   *viper.Viper
	logger  *zap.Logger
}

// AddFlags implements plugin.Configurable
func (f *configurable) AddFlags(flagSet *flag.FlagSet) {
	f.flagSet = flagSet
}

// InitFromViper implements plugin.Configurable
func (f *configurable) InitFromViper(v *viper.Viper, logger *zap.Logger) {
	f.viper = v
	f.logger = logger
}

func TestConfigurable(t *testing.T) {
	f := NewFactory(defaultCfg())

	mock := new(configurable)
	f.factories[cassandraStorageType] = mock

	fs := new(flag.FlagSet)
	v := viper.New()

	f.AddFlags(fs)
	f.InitFromViper(v, zap.NewNop())

	assert.Equal(t, fs, mock.flagSet)
	assert.Equal(t, v, mock.viper)
}

func TestConfigurablePanic(t *testing.T) {
	cfg := FactoryConfig{
		SpanWriterTypes:         []string{"x"},
		SpanReaderType:          "x",
		DependenciesStorageType: "x",
		DownsamplingRatio:       1.0,
		DownsamplingHashSalt:    "",
	}
	f := NewFactory(cfg)
	var flagSet flag.FlagSet
	assert.PanicsWithError(t,
		"unknown storage type x. Valid types are [cassandra opensearch elasticsearch memory kafka badger blackhole grpc]",
		func() { f.AddFlags(&flagSet) },
	)
}

func TestParsingDownsamplingRatio(t *testing.T) {
	f := Factory{}
	v, command := config.Viperize(f.AddPipelineFlags)
	err := command.ParseFlags([]string{
		"--downsampling.ratio=1.5",
		"--downsampling.hashsalt=jaeger",
	})
	require.NoError(t, err)
	f.InitFromViper(v, zap.NewNop())

	assert.InDelta(t, 1.0, f.FactoryConfig.DownsamplingRatio, 0.01)
	assert.Equal(t, "jaeger", f.FactoryConfig.DownsamplingHashSalt)

	err = command.ParseFlags([]string{
		"--downsampling.ratio=0.5",
	})
	require.NoError(t, err)
	f.InitFromViper(v, zap.NewNop())
	assert.InDelta(t, 0.5, f.FactoryConfig.DownsamplingRatio, 0.01)
}

func TestDefaultDownsamplingWithAddFlags(t *testing.T) {
	f := Factory{}
	v, command := config.Viperize(f.AddFlags)
	err := command.ParseFlags([]string{})
	require.NoError(t, err)
	f.InitFromViper(v, zap.NewNop())

	assert.InDelta(t, defaultDownsamplingRatio, f.FactoryConfig.DownsamplingRatio, 0.01)
	assert.Equal(t, defaultDownsamplingHashSalt, f.FactoryConfig.DownsamplingHashSalt)

	err = command.ParseFlags([]string{
		"--downsampling.ratio=0.5",
	})
	require.Error(t, err)
}

func TestPublishOpts(t *testing.T) {
	f := NewFactory(defaultCfg())
	f.publishOpts()
	assert.EqualValues(t, 1, expvar.Get(downsamplingRatio).(*expvar.Int).Value())
	assert.EqualValues(t, 1, expvar.Get(spanStorageType+"-"+f.SpanReaderType).(*expvar.Int).Value())
}

type errorFactory struct {
	closeErr error
}

var (
	_ storage.Factory = (*errorFactory)(nil)
	_ io.Closer       = (*errorFactory)(nil)
)

func (errorFactory) Initialize() error {
	panic("implement me")
}

func (errorFactory) CreateSpanReader() (spanstore.Reader, error) {
	panic("implement me")
}

func (errorFactory) CreateSpanWriter() (spanstore.Writer, error) {
	panic("implement me")
}

func (errorFactory) CreateDependencyReader() (dependencystore.Reader, error) {
	panic("implement me")
}

func (e errorFactory) Close() error {
	return e.closeErr
}
