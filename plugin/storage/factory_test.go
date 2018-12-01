// Copyright (c) 2018 Uber Technologies, Inc.
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

package storage

import (
	"errors"
	"flag"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/storage"
	depStoreMocks "github.com/jaegertracing/jaeger/storage/dependencystore/mocks"
	"github.com/jaegertracing/jaeger/storage/mocks"
	"github.com/jaegertracing/jaeger/storage/spanstore"
	spanStoreMocks "github.com/jaegertracing/jaeger/storage/spanstore/mocks"
)

var _ storage.Factory = new(Factory)
var _ storage.ArchiveFactory = new(Factory)

func defaultCfg() FactoryConfig {
	return FactoryConfig{
		SpanWriterTypes:         []string{CassandraStorageType},
		SpanReaderType:          CassandraStorageType,
		DependenciesStorageType: CassandraStorageType,
	}
}

func TestNewFactory(t *testing.T) {
	f, err := NewFactory(defaultCfg(), nil)
	require.NoError(t, err)
	assert.NotEmpty(t, f.factories)
	assert.NotEmpty(t, f.factories[CassandraStorageType])
	assert.Equal(t, CassandraStorageType, f.SpanWriterTypes[0])
	assert.Equal(t, CassandraStorageType, f.SpanReaderType)
	assert.Equal(t, CassandraStorageType, f.DependenciesStorageType)

	f, err = NewFactory(FactoryConfig{
		SpanWriterTypes:         []string{CassandraStorageType, KafkaStorageType},
		SpanReaderType:          ElasticsearchStorageType,
		DependenciesStorageType: MemoryStorageType,
	}, nil)
	require.NoError(t, err)
	assert.NotEmpty(t, f.factories)
	assert.NotEmpty(t, f.factories[CassandraStorageType])
	assert.NotNil(t, f.factories[KafkaStorageType])
	assert.NotEmpty(t, f.factories[ElasticsearchStorageType])
	assert.NotNil(t, f.factories[MemoryStorageType])
	assert.Equal(t, []string{CassandraStorageType, KafkaStorageType}, f.SpanWriterTypes)
	assert.Equal(t, ElasticsearchStorageType, f.SpanReaderType)
	assert.Equal(t, MemoryStorageType, f.DependenciesStorageType)

	f, err = NewFactory(FactoryConfig{SpanWriterTypes: []string{"x"}, DependenciesStorageType: "y", SpanReaderType: "z"}, nil)
	require.Error(t, err)
	expected := "Unknown storage type" // could be 'x' or 'y' since code iterates through map.
	assert.Equal(t, expected, err.Error()[0:len(expected)])
}

func TestNewFactoryWithUnsupportedType(t *testing.T) {
	_, err := NewFactory(defaultCfg(), []string{CassandraStorageType})

	assert.EqualError(t, err, "The cassandra storage type is unsupported by this command")
}

func TestInitialize(t *testing.T) {
	f, err := NewFactory(defaultCfg(), nil)
	require.NoError(t, err)
	assert.NotEmpty(t, f.factories)
	assert.NotEmpty(t, f.factories[CassandraStorageType])

	mock := new(mocks.Factory)
	f.factories[CassandraStorageType] = mock

	m := metrics.NullFactory
	l := zap.NewNop()
	mock.On("Initialize", m, l).Return(nil)
	assert.NoError(t, f.Initialize(m, l))

	mock = new(mocks.Factory)
	f.factories[CassandraStorageType] = mock
	mock.On("Initialize", m, l).Return(errors.New("init-error"))
	assert.EqualError(t, f.Initialize(m, l), "init-error")
}

func TestCreate(t *testing.T) {
	f, err := NewFactory(defaultCfg(), nil)
	require.NoError(t, err)
	assert.NotEmpty(t, f.factories)
	assert.NotEmpty(t, f.factories[CassandraStorageType])

	mock := new(mocks.Factory)
	f.factories[CassandraStorageType] = mock

	spanReader := new(spanStoreMocks.Reader)
	spanWriter := new(spanStoreMocks.Writer)
	depReader := new(depStoreMocks.Reader)

	mock.On("CreateSpanReader").Return(spanReader, errors.New("span-reader-error"))
	mock.On("CreateSpanWriter").Once().Return(spanWriter, errors.New("span-writer-error"))
	mock.On("CreateDependencyReader").Return(depReader, errors.New("dep-reader-error"))

	r, err := f.CreateSpanReader()
	assert.Equal(t, spanReader, r)
	assert.EqualError(t, err, "span-reader-error")

	w, err := f.CreateSpanWriter()
	assert.Nil(t, w)
	assert.EqualError(t, err, "span-writer-error")

	d, err := f.CreateDependencyReader()
	assert.Equal(t, depReader, d)
	assert.EqualError(t, err, "dep-reader-error")

	_, err = f.CreateArchiveSpanReader()
	assert.EqualError(t, err, "Archive storage not supported")

	_, err = f.CreateArchiveSpanWriter()
	assert.EqualError(t, err, "Archive storage not supported")

	mock.On("CreateSpanWriter").Return(spanWriter, nil)
	w, err = f.CreateSpanWriter()
	assert.NoError(t, err)
	assert.Equal(t, spanWriter, w)
}

func TestCreateMulti(t *testing.T) {
	cfg := defaultCfg()
	cfg.SpanWriterTypes = append(cfg.SpanWriterTypes, ElasticsearchStorageType)
	f, err := NewFactory(cfg, nil)
	require.NoError(t, err)

	mock := new(mocks.Factory)
	mock2 := new(mocks.Factory)
	f.factories[CassandraStorageType] = mock
	f.factories[ElasticsearchStorageType] = mock2

	spanWriter := new(spanStoreMocks.Writer)
	spanWriter2 := new(spanStoreMocks.Writer)

	mock.On("CreateSpanWriter").Once().Return(spanWriter, errors.New("span-writer-error"))

	w, err := f.CreateSpanWriter()
	assert.Nil(t, w)
	assert.EqualError(t, err, "span-writer-error")

	mock.On("CreateSpanWriter").Return(spanWriter, nil)
	mock2.On("CreateSpanWriter").Return(spanWriter2, nil)
	w, err = f.CreateSpanWriter()
	assert.NoError(t, err)
	assert.Equal(t, spanstore.NewCompositeWriter(spanWriter, spanWriter2), w)
}

func TestCreateArchive(t *testing.T) {
	f, err := NewFactory(defaultCfg(), nil)
	require.NoError(t, err)
	assert.NotEmpty(t, f.factories[CassandraStorageType])

	mock := &struct {
		mocks.Factory
		mocks.ArchiveFactory
	}{}
	f.factories[CassandraStorageType] = mock

	archiveSpanReader := new(spanStoreMocks.Reader)
	archiveSpanWriter := new(spanStoreMocks.Writer)

	mock.ArchiveFactory.On("CreateArchiveSpanReader").Return(archiveSpanReader, errors.New("archive-span-reader-error"))
	mock.ArchiveFactory.On("CreateArchiveSpanWriter").Return(archiveSpanWriter, errors.New("archive-span-writer-error"))

	ar, err := f.CreateArchiveSpanReader()
	assert.Equal(t, archiveSpanReader, ar)
	assert.EqualError(t, err, "archive-span-reader-error")

	aw, err := f.CreateArchiveSpanWriter()
	assert.Equal(t, archiveSpanWriter, aw)
	assert.EqualError(t, err, "archive-span-writer-error")
}

func TestCreateError(t *testing.T) {
	f, err := NewFactory(defaultCfg(), nil)
	require.NoError(t, err)
	assert.NotEmpty(t, f.factories)
	assert.NotEmpty(t, f.factories[CassandraStorageType])
	delete(f.factories, CassandraStorageType)

	expectedErr := "No cassandra backend registered for span store"
	// scope the vars to avoid bugs in the test
	{
		r, err := f.CreateSpanReader()
		assert.Nil(t, r)
		assert.EqualError(t, err, expectedErr)
	}

	{
		w, err := f.CreateSpanWriter()
		assert.Nil(t, w)
		assert.EqualError(t, err, expectedErr)
	}

	{
		d, err := f.CreateDependencyReader()
		assert.Nil(t, d)
		assert.EqualError(t, err, expectedErr)
	}

	{
		r, err := f.CreateArchiveSpanReader()
		assert.Nil(t, r)
		assert.EqualError(t, err, expectedErr)
	}

	{
		w, err := f.CreateArchiveSpanWriter()
		assert.Nil(t, w)
		assert.EqualError(t, err, expectedErr)
	}
}

type configurable struct {
	mocks.Factory
	flagSet *flag.FlagSet
	viper   *viper.Viper
}

// AddFlags implements plugin.Configurable
func (f *configurable) AddFlags(flagSet *flag.FlagSet) {
	f.flagSet = flagSet
}

// InitFromViper implements plugin.Configurable
func (f *configurable) InitFromViper(v *viper.Viper) {
	f.viper = v
}

func TestConfigurable(t *testing.T) {
	clearEnv()
	defer clearEnv()

	f, err := NewFactory(defaultCfg(), nil)
	require.NoError(t, err)
	assert.NotEmpty(t, f.factories)
	assert.NotEmpty(t, f.factories[CassandraStorageType])

	mock := new(configurable)
	f.factories[CassandraStorageType] = mock

	fs := new(flag.FlagSet)
	v := viper.New()

	f.AddFlags(fs)
	f.InitFromViper(v)

	assert.Equal(t, fs, mock.flagSet)
	assert.Equal(t, v, mock.viper)
}
