// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package v1adapter

import (
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/require"

	storage_v1 "github.com/jaegertracing/jaeger/internal/storage/v1"
	dependencyStoreMocks "github.com/jaegertracing/jaeger/internal/storage/v1/api/dependencystore/mocks"
	spanstoreMocks "github.com/jaegertracing/jaeger/internal/storage/v1/api/spanstore/mocks"
	factoryMocks "github.com/jaegertracing/jaeger/internal/storage/v1/mocks"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/depstore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

func TestNewFactory(t *testing.T) {
	mockFactory := new(factoryMocks.Factory)
	mockPurger := new(factoryMocks.Purger)
	mockSamplingStoreFactory := new(factoryMocks.SamplingStoreFactory)

	tests := []struct {
		name               string
		factory            storage_v1.Factory
		expectedInterfaces []any
	}{
		{
			name:    "No extra interfaces",
			factory: mockFactory,
			expectedInterfaces: []any{
				(*tracestore.Factory)(nil),
				(*depstore.Factory)(nil),
				(*io.Closer)(nil)},
		},
		{
			name: "Implements Purger",
			factory: struct {
				storage_v1.Factory
				storage_v1.Purger
			}{mockFactory, mockPurger},
			expectedInterfaces: []any{
				(*tracestore.Factory)(nil),
				(*depstore.Factory)(nil),
				(*io.Closer)(nil),
				(*storage_v1.Purger)(nil),
			},
		},
		{
			name: "Implements SamplingStoreFactory",
			factory: struct {
				storage_v1.Factory
				storage_v1.SamplingStoreFactory
			}{mockFactory, mockSamplingStoreFactory},
			expectedInterfaces: []any{
				(*tracestore.Factory)(nil),
				(*depstore.Factory)(nil),
				(*io.Closer)(nil),
				(*storage_v1.SamplingStoreFactory)(nil),
			},
		},
		{
			name: "Implements both Purger and SamplingStoreFactory",
			factory: struct {
				storage_v1.Factory
				storage_v1.Purger
				storage_v1.SamplingStoreFactory
			}{mockFactory, mockPurger, mockSamplingStoreFactory},
			expectedInterfaces: []any{
				(*tracestore.Factory)(nil),
				(*depstore.Factory)(nil),
				(*io.Closer)(nil),
				(*storage_v1.Purger)(nil),
				(*storage_v1.SamplingStoreFactory)(nil),
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			traceReader := NewFactory(test.factory)
			for _, i := range test.expectedInterfaces {
				require.Implements(t, i, traceReader)
			}
		})
	}
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
	depFactory, ok := f.(depstore.Factory)
	require.True(t, ok)
	r, err := depFactory.CreateDependencyReader()
	require.NoError(t, err)
	require.NotNil(t, r)
}

func TestAdapterCreateDependencyReaderError(t *testing.T) {
	f1 := new(factoryMocks.Factory)
	testErr := errors.New("test error")
	f1.On("CreateDependencyReader").Return(nil, testErr)

	f := NewFactory(f1)
	depFactory, ok := f.(depstore.Factory)
	require.True(t, ok)
	r, err := depFactory.CreateDependencyReader()
	require.ErrorIs(t, err, testErr)
	require.Nil(t, r)
}
