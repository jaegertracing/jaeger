// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2018 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package factory

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

	"github.com/jaegertracing/jaeger/internal/config"
	"github.com/jaegertracing/jaeger/internal/metrics"
	"github.com/jaegertracing/jaeger/internal/storage/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/dependencystore"
	depStoreMocks "github.com/jaegertracing/jaeger/internal/storage/v1/api/dependencystore/mocks"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/spanstore"
	spanStoreMocks "github.com/jaegertracing/jaeger/internal/storage/v1/api/spanstore/mocks"
	"github.com/jaegertracing/jaeger/internal/storage/v1/mocks"
)

func defaultCfg() Config {
	return Config{
		SpanWriterTypes:         []string{cassandraStorageType},
		SpanReaderType:          cassandraStorageType,
		DependenciesStorageType: cassandraStorageType,
		DownsamplingRatio:       1.0,
		DownsamplingHashSalt:    "",
	}
}

func TestNewFactory(t *testing.T) {
	f, err := NewFactory(defaultCfg())
	require.NoError(t, err)
	assert.NotEmpty(t, f.factories)
	assert.NotEmpty(t, f.factories[cassandraStorageType])
	assert.Equal(t, cassandraStorageType, f.SpanWriterTypes[0])
	assert.Equal(t, cassandraStorageType, f.SpanReaderType)
	assert.Equal(t, cassandraStorageType, f.DependenciesStorageType)

	f, err = NewFactory(Config{
		SpanWriterTypes:         []string{cassandraStorageType, kafkaStorageType, badgerStorageType},
		SpanReaderType:          elasticsearchStorageType,
		DependenciesStorageType: memoryStorageType,
	})
	require.NoError(t, err)
	assert.NotEmpty(t, f.factories)
	assert.NotEmpty(t, f.factories[cassandraStorageType])
	assert.NotNil(t, f.factories[kafkaStorageType])
	assert.NotEmpty(t, f.factories[elasticsearchStorageType])
	assert.NotNil(t, f.factories[memoryStorageType])
	assert.Equal(t, []string{cassandraStorageType, kafkaStorageType, badgerStorageType}, f.SpanWriterTypes)
	assert.Equal(t, elasticsearchStorageType, f.SpanReaderType)
	assert.Equal(t, memoryStorageType, f.DependenciesStorageType)

	_, err = NewFactory(Config{SpanWriterTypes: []string{"x"}, DependenciesStorageType: "y", SpanReaderType: "z"})
	require.Error(t, err)
	expected := "unknown storage type" // could be 'x' or 'y' since code iterates through map.
	assert.Equal(t, expected, err.Error()[0:len(expected)])

	require.NoError(t, f.Close())
}

func TestClose(t *testing.T) {
	storageType := "foo"
	err := errors.New("some error")
	f := Factory{
		factories: map[string]storage.Factory{
			storageType: &errorFactory{closeErr: err},
		},
		Config: Config{SpanWriterTypes: []string{storageType}},
	}
	require.EqualError(t, f.Close(), err.Error())
}

func TestInitialize(t *testing.T) {
	f, err := NewFactory(defaultCfg())
	require.NoError(t, err)
	assert.NotEmpty(t, f.factories)
	assert.NotEmpty(t, f.factories[cassandraStorageType])

	mock := new(mocks.Factory)
	f.factories[cassandraStorageType] = mock
	f.archiveFactories[cassandraStorageType] = mock

	m := metrics.NullFactory
	l := zap.NewNop()
	mock.On("Initialize", m, l).Return(nil)
	require.NoError(t, f.Initialize(m, l))

	mock = new(mocks.Factory)
	f.factories[cassandraStorageType] = mock
	mock.On("Initialize", m, l).Return(errors.New("init-error"))
	require.EqualError(t, f.Initialize(m, l), "init-error")
}

func TestCreate(t *testing.T) {
	f, err := NewFactory(defaultCfg())
	require.NoError(t, err)
	assert.NotEmpty(t, f.factories)
	assert.NotEmpty(t, f.factories[cassandraStorageType])

	mock := new(mocks.Factory)
	f.factories[cassandraStorageType] = mock

	spanReader := new(spanStoreMocks.Reader)
	spanWriter := new(spanStoreMocks.Writer)
	depReader := new(depStoreMocks.Reader)

	mock.On("CreateSpanReader").Return(spanReader, errors.New("span-reader-error"))
	mock.On("CreateSpanWriter").Once().Return(spanWriter, errors.New("span-writer-error"))
	mock.On("CreateDependencyReader").Return(depReader, errors.New("dep-reader-error"))

	r, err := f.CreateSpanReader()
	assert.Equal(t, spanReader, r)
	require.EqualError(t, err, "span-reader-error")

	w, err := f.CreateSpanWriter()
	assert.Nil(t, w)
	require.EqualError(t, err, "span-writer-error")

	d, err := f.CreateDependencyReader()
	assert.Equal(t, depReader, d)
	require.EqualError(t, err, "dep-reader-error")

	mock.On("CreateSpanWriter").Return(spanWriter, nil)
	m := metrics.NullFactory
	l := zap.NewNop()
	mock.On("Initialize", m, l).Return(nil)
	f.Initialize(m, l)
	w, err = f.CreateSpanWriter()
	require.NoError(t, err)
	assert.Equal(t, spanWriter, w)
}

func TestCreateDownsamplingWriter(t *testing.T) {
	f, err := NewFactory(defaultCfg())
	require.NoError(t, err)
	assert.NotEmpty(t, f.factories[cassandraStorageType])
	mock := new(mocks.Factory)
	f.factories[cassandraStorageType] = mock
	spanWriter := new(spanStoreMocks.Writer)
	mock.On("CreateSpanWriter").Return(spanWriter, nil)

	m := metrics.NullFactory
	l := zap.NewNop()
	mock.On("Initialize", m, l).Return(nil)

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
			f.Initialize(m, l)
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
	f, err := NewFactory(cfg)
	require.NoError(t, err)

	mock := new(mocks.Factory)
	mock2 := new(mocks.Factory)
	f.factories[cassandraStorageType] = mock
	f.archiveFactories[cassandraStorageType] = mock
	f.factories[elasticsearchStorageType] = mock2
	f.archiveFactories[elasticsearchStorageType] = mock2

	spanWriter := new(spanStoreMocks.Writer)
	spanWriter2 := new(spanStoreMocks.Writer)

	mock.On("CreateSpanWriter").Once().Return(spanWriter, errors.New("span-writer-error"))

	w, err := f.CreateSpanWriter()
	assert.Nil(t, w)
	require.EqualError(t, err, "span-writer-error")

	mock.On("CreateSpanWriter").Return(spanWriter, nil)
	mock2.On("CreateSpanWriter").Return(spanWriter2, nil)
	m := metrics.NullFactory
	l := zap.NewNop()
	mock.On("Initialize", m, l).Return(nil)
	mock2.On("Initialize", m, l).Return(nil)
	f.Initialize(m, l)
	w, err = f.CreateSpanWriter()
	require.NoError(t, err)
	assert.Equal(t, spanstore.NewCompositeWriter(spanWriter, spanWriter2), w)
}

func TestCreateError(t *testing.T) {
	f, err := NewFactory(defaultCfg())
	require.NoError(t, err)
	assert.NotEmpty(t, f.factories)
	assert.NotEmpty(t, f.factories[cassandraStorageType])
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
}

func TestAllSamplingStorageTypes(t *testing.T) {
	assert.Equal(t, []string{"cassandra", "memory", "badger"}, AllSamplingStorageTypes())
}

func TestCreateSamplingStoreFactory(t *testing.T) {
	f, err := NewFactory(defaultCfg())
	require.NoError(t, err)
	assert.NotEmpty(t, f.factories)
	assert.NotEmpty(t, f.factories[cassandraStorageType])

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
	f, err = NewFactory(cfg)
	require.NoError(t, err)
	ssFactory, err = f.CreateSamplingStoreFactory()
	assert.Nil(t, ssFactory)
	require.EqualError(t, err, "storage factory of type elasticsearch does not support sampling store")

	// if a compatible factory is specified then return it
	cfg.SamplingStorageType = "cassandra"
	f, err = NewFactory(cfg)
	require.NoError(t, err)
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

// AddFlags implements storage.Configurable
func (f *configurable) AddFlags(flagSet *flag.FlagSet) {
	f.flagSet = flagSet
}

// InitFromViper implements storage.Configurable
func (f *configurable) InitFromViper(v *viper.Viper, logger *zap.Logger) {
	f.viper = v
	f.logger = logger
}

func TestConfigurable(t *testing.T) {
	f, err := NewFactory(defaultCfg())
	require.NoError(t, err)
	assert.NotEmpty(t, f.factories)
	assert.NotEmpty(t, f.factories[cassandraStorageType])

	mock := new(configurable)
	f.factories[cassandraStorageType] = mock

	fs := new(flag.FlagSet)
	v := viper.New()

	f.AddFlags(fs)
	f.InitFromViper(v, zap.NewNop())

	assert.Equal(t, fs, mock.flagSet)
	assert.Equal(t, v, mock.viper)
}

type inheritable struct {
	mocks.Factory
	calledWith storage.Factory
}

func (i *inheritable) InheritSettingsFrom(other storage.Factory) {
	i.calledWith = other
}

func TestInheritable(t *testing.T) {
	f, err := NewFactory(defaultCfg())
	require.NoError(t, err)
	assert.NotEmpty(t, f.factories)
	assert.NotEmpty(t, f.factories[cassandraStorageType])
	assert.NotEmpty(t, f.archiveFactories[cassandraStorageType])

	mockInheritable := new(inheritable)
	f.factories[cassandraStorageType] = &mocks.Factory{}
	f.archiveFactories[cassandraStorageType] = mockInheritable

	fs := new(flag.FlagSet)
	v := viper.New()

	f.AddFlags(fs)
	f.InitFromViper(v, zap.NewNop())

	assert.Equal(t, f.factories[cassandraStorageType], mockInheritable.calledWith)
}

type archiveConfigurable struct {
	isConfigurable bool
	*mocks.Factory
}

func (ac *archiveConfigurable) IsArchiveCapable() bool {
	return ac.isConfigurable
}

func TestArchiveConfigurable(t *testing.T) {
	tests := []struct {
		name                string
		isArchiveCapable    bool
		archiveInitError    error
		expectedError       error
		expectedArchiveSize int
	}{
		{
			name:                "Archive factory initializes successfully",
			isArchiveCapable:    true,
			archiveInitError:    nil,
			expectedError:       nil,
			expectedArchiveSize: 1,
		},
		{
			name:                "Archive factory initialization fails",
			isArchiveCapable:    true,
			archiveInitError:    assert.AnError,
			expectedError:       assert.AnError,
			expectedArchiveSize: 1,
		},
		{
			name:                "Archive factory is not archive-capable",
			isArchiveCapable:    false,
			archiveInitError:    nil,
			expectedError:       nil,
			expectedArchiveSize: 0,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			f, err := NewFactory(defaultCfg())
			require.NoError(t, err)

			primaryFactory := &mocks.Factory{}
			archiveFactory := &mocks.Factory{}
			archiveConfigurable := &archiveConfigurable{
				isConfigurable: test.isArchiveCapable,
				Factory:        archiveFactory,
			}

			f.factories[cassandraStorageType] = primaryFactory
			f.archiveFactories[cassandraStorageType] = archiveConfigurable

			m := metrics.NullFactory
			l := zap.NewNop()
			primaryFactory.On("Initialize", m, l).Return(nil).Once()
			archiveFactory.On("Initialize", m, l).Return(test.archiveInitError).Once()

			err = f.Initialize(m, l)
			if test.expectedError != nil {
				require.ErrorIs(t, err, test.expectedError)
			} else {
				require.NoError(t, err)
			}

			assert.Len(t, f.archiveFactories, test.expectedArchiveSize)
		})
	}
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

	assert.InDelta(t, 1.0, f.DownsamplingRatio, 0.01)
	assert.Equal(t, "jaeger", f.DownsamplingHashSalt)

	err = command.ParseFlags([]string{
		"--downsampling.ratio=0.5",
	})
	require.NoError(t, err)
	f.InitFromViper(v, zap.NewNop())
	assert.InDelta(t, 0.5, f.DownsamplingRatio, 0.01)
}

func TestDefaultDownsamplingWithAddFlags(t *testing.T) {
	f := Factory{}
	v, command := config.Viperize(f.AddFlags)
	err := command.ParseFlags([]string{})
	require.NoError(t, err)
	f.InitFromViper(v, zap.NewNop())

	assert.InDelta(t, defaultDownsamplingRatio, f.DownsamplingRatio, 0.01)
	assert.Equal(t, defaultDownsamplingHashSalt, f.DownsamplingHashSalt)

	err = command.ParseFlags([]string{
		"--downsampling.ratio=0.5",
	})
	require.Error(t, err)
}

func TestPublishOpts(t *testing.T) {
	f, err := NewFactory(defaultCfg())
	require.NoError(t, err)
	f.publishOpts()

	assert.EqualValues(t, 1, expvar.Get(downsamplingRatio).(*expvar.Int).Value())
	assert.EqualValues(t, 1, expvar.Get(spanStorageType+"-"+f.SpanReaderType).(*expvar.Int).Value())
}

func TestInitArchiveStorage(t *testing.T) {
	tests := []struct {
		name            string
		setupMock       func(*mocks.Factory)
		factoryCfg      func() Config
		expectedStorage *ArchiveStorage
		expectedError   error
	}{
		{
			name: "successful initialization",
			setupMock: func(mock *mocks.Factory) {
				spanReader := &spanStoreMocks.Reader{}
				spanWriter := &spanStoreMocks.Writer{}
				mock.On("CreateSpanReader").Return(spanReader, nil)
				mock.On("CreateSpanWriter").Return(spanWriter, nil)
			},
			factoryCfg: defaultCfg,
			expectedStorage: &ArchiveStorage{
				Reader: &spanStoreMocks.Reader{},
				Writer: &spanStoreMocks.Writer{},
			},
		},
		{
			name: "no archive span reader",
			setupMock: func(mock *mocks.Factory) {
				spanReader := &spanStoreMocks.Reader{}
				spanWriter := &spanStoreMocks.Writer{}
				mock.On("CreateSpanReader").Return(spanReader, nil)
				mock.On("CreateSpanWriter").Return(spanWriter, nil)
			},
			factoryCfg: func() Config {
				cfg := defaultCfg()
				cfg.SpanReaderType = "blackhole"
				return cfg
			},
			expectedStorage: nil,
		},
		{
			name: "no archive span writer",
			setupMock: func(mock *mocks.Factory) {
				spanReader := &spanStoreMocks.Reader{}
				spanWriter := &spanStoreMocks.Writer{}
				mock.On("CreateSpanReader").Return(spanReader, nil)
				mock.On("CreateSpanWriter").Return(spanWriter, nil)
			},
			factoryCfg: func() Config {
				cfg := defaultCfg()
				cfg.SpanWriterTypes = []string{"blackhole"}
				return cfg
			},
			expectedStorage: nil,
		},
		{
			name: "error initializing reader",
			setupMock: func(mock *mocks.Factory) {
				mock.On("CreateSpanReader").Return(nil, assert.AnError)
			},
			factoryCfg:      defaultCfg,
			expectedStorage: nil,
			expectedError:   assert.AnError,
		},
		{
			name: "error initializing writer",
			setupMock: func(mock *mocks.Factory) {
				spanReader := new(spanStoreMocks.Reader)
				mock.On("CreateSpanReader").Return(spanReader, nil)
				mock.On("CreateSpanWriter").Return(nil, assert.AnError)
			},
			factoryCfg:      defaultCfg,
			expectedStorage: nil,
			expectedError:   assert.AnError,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := test.factoryCfg()
			f, err := NewFactory(cfg)
			require.NoError(t, err)

			mock := new(mocks.Factory)
			f.archiveFactories[cassandraStorageType] = mock
			test.setupMock(mock)

			storage, err := f.InitArchiveStorage()
			require.Equal(t, test.expectedStorage, storage)
			require.ErrorIs(t, err, test.expectedError)
		})
	}
}

type errorFactory struct {
	closeErr error
}

var (
	_ storage.Factory = (*errorFactory)(nil)
	_ io.Closer       = (*errorFactory)(nil)
)

func (errorFactory) Initialize(metrics.Factory, *zap.Logger) error {
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
