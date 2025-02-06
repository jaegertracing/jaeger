// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package factory

import (
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/internal/storage/v2"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	traceStoreMocks "github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore/mocks"
	"github.com/jaegertracing/jaeger/internal/storage/v2/mocks"
)

func defaultCfg() Config {
	return Config{
		TraceWriterTypes: []string{clickhouseStorageType},
	}
}

func TestNewFactory(t *testing.T) {
	f, err := NewFactory(defaultCfg())
	require.NoError(t, err)
	assert.NotEmpty(t, f.factories)
	// TODO use assert.NotEmpty() after implement clickhouse
	assert.NotNil(t, f.factories[clickhouseStorageType])
	assert.Equal(t, clickhouseStorageType, f.TraceWriterTypes[0])

	_, err = NewFactory(Config{TraceWriterTypes: []string{"not a real traceWriter type"}})
	expected := "unknown storage type"
	require.Error(t, err)
	assert.Equal(t, expected, err.Error()[0:len(expected)])

	require.NoError(t, f.Close())
}

func TestCreate(t *testing.T) {
	f, err := NewFactory(defaultCfg())
	require.NoError(t, err)
	// TODO use assert.NotEmpty() after implement clickhouse
	require.NotNil(t, f)
	assert.NotNil(t, f.factories[clickhouseStorageType])

	mock := new(mocks.Factory)
	f.factories[clickhouseStorageType] = mock

	traceWriter := new(traceStoreMocks.Writer)

	mock.On("CreateTraceWriter").Return(traceWriter, errors.New("trace-writer-error"))

	w, err := f.CreatTraceWriter()
	assert.Nil(t, w)
	require.EqualError(t, err, "trace-writer-error")
}

func TestCreateError(t *testing.T) {
	f, err := NewFactory(defaultCfg())
	require.NoError(t, err)
	require.NotNil(t, f)
	assert.NotNil(t, f.factories[clickhouseStorageType])
	delete(f.factories, clickhouseStorageType)
	expectedErr := "no clickhouse backend registered for trace store"

	w, err := f.CreatTraceWriter()
	assert.Nil(t, w)
	require.EqualError(t, err, expectedErr)
}

func TestClose(t *testing.T) {
	storageType := "foo"
	err := errors.New("some error")
	f := Factory{
		factories: map[string]storage.Factory{
			storageType: &errorFactory{closeErr: err},
		},
		Config: Config{TraceWriterTypes: []string{storageType}},
	}
	require.EqualError(t, f.Close(), err.Error())
}

type errorFactory struct {
	closeErr error
}

var (
	_ storage.Factory = (*errorFactory)(nil)
	_ io.Closer       = (*errorFactory)(nil)
)

func (errorFactory) CreateTraceWriter() (tracestore.Writer, error) {
	panic("implement me")
}

func (e errorFactory) Close() error {
	return e.closeErr
}
