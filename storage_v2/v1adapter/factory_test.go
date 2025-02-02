// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package v1adapter

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	dependencyStoreMocks "github.com/jaegertracing/jaeger/internal/storage/v1/api/dependencystore/mocks"
	factoryMocks "github.com/jaegertracing/jaeger/internal/storage/v1/api/mocks"
	spanstoreMocks "github.com/jaegertracing/jaeger/internal/storage/v1/api/spanstore/mocks"
	"github.com/jaegertracing/jaeger/internal/storage/v1/grpc"
)

func TestAdapterInitialize(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("initialize did not panic")
		}
	}()

	f := &Factory{}
	_ = f.Initialize(context.Background())
}

func TestAdapterCloseNotOk(t *testing.T) {
	f := NewFactory(&factoryMocks.Factory{})
	require.NoError(t, f.Close(context.Background()))
}

func TestAdapterClose(t *testing.T) {
	f := NewFactory(grpc.NewFactory())
	require.NoError(t, f.Close(context.Background()))
}

func TestAdapterCreateTraceReader(t *testing.T) {
	f1 := new(factoryMocks.Factory)
	f1.On("CreateSpanReader").Return(new(spanstoreMocks.Reader), nil)

	f := NewFactory(f1)
	_, err := f.CreateTraceReader()
	require.NoError(t, err)
}

func TestAdapterCreateTraceReaderError(t *testing.T) {
	f1 := new(factoryMocks.Factory)
	f1.On("CreateSpanReader").Return(nil, errors.New("mock error"))

	f := NewFactory(f1)
	_, err := f.CreateTraceReader()
	require.ErrorContains(t, err, "mock error")
}

func TestAdapterCreateTraceWriterError(t *testing.T) {
	f1 := new(factoryMocks.Factory)
	f1.On("CreateSpanWriter").Return(nil, errors.New("mock error"))

	f := NewFactory(f1)
	_, err := f.CreateTraceWriter()
	require.ErrorContains(t, err, "mock error")
}

func TestAdapterCreateTraceWriter(t *testing.T) {
	f1 := new(factoryMocks.Factory)
	f1.On("CreateSpanWriter").Return(new(spanstoreMocks.Writer), nil)

	f := NewFactory(f1)
	_, err := f.CreateTraceWriter()
	require.NoError(t, err)
}

func TestAdapterCreateDependencyReader(t *testing.T) {
	f1 := new(factoryMocks.Factory)
	f1.On("CreateDependencyReader").Return(new(dependencyStoreMocks.Reader), nil)

	f := NewFactory(f1)
	r, err := f.CreateDependencyReader()
	require.NoError(t, err)
	require.NotNil(t, r)
}

func TestAdapterCreateDependencyReaderError(t *testing.T) {
	f1 := new(factoryMocks.Factory)
	testErr := errors.New("test error")
	f1.On("CreateDependencyReader").Return(nil, testErr)

	f := NewFactory(f1)
	r, err := f.CreateDependencyReader()
	require.ErrorIs(t, err, testErr)
	require.Nil(t, r)
}
