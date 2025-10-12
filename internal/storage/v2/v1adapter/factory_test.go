// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package v1adapter

import (
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/require"

	storagev1 "github.com/jaegertracing/jaeger/internal/storage/v1"
	dependencystoremocks "github.com/jaegertracing/jaeger/internal/storage/v1/api/dependencystore/mocks"
	spanstoremocks "github.com/jaegertracing/jaeger/internal/storage/v1/api/spanstore/mocks"
	"github.com/jaegertracing/jaeger/internal/storage/v1/grpc"
	factorymocks "github.com/jaegertracing/jaeger/internal/storage/v1/mocks"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/depstore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
)

func TestNewFactory(t *testing.T) {
	mockFactory := new(factorymocks.Factory)
	mockPurger := new(factorymocks.Purger)
	mockSamplingStoreFactory := new(factorymocks.SamplingStoreFactory)

	tests := []struct {
		name               string
		factory            storagev1.Factory
		expectedInterfaces []any
	}{
		{
			name:    "No extra interfaces",
			factory: mockFactory,
			expectedInterfaces: []any{
				(*tracestore.Factory)(nil),
				(*depstore.Factory)(nil),
				(*io.Closer)(nil),
			},
		},
		{
			name: "Implements Purger",
			factory: struct {
				storagev1.Factory
				storagev1.Purger
			}{mockFactory, mockPurger},
			expectedInterfaces: []any{
				(*tracestore.Factory)(nil),
				(*depstore.Factory)(nil),
				(*io.Closer)(nil),
				(*storagev1.Purger)(nil),
			},
		},
		{
			name: "Implements SamplingStoreFactory",
			factory: struct {
				storagev1.Factory
				storagev1.SamplingStoreFactory
			}{mockFactory, mockSamplingStoreFactory},
			expectedInterfaces: []any{
				(*tracestore.Factory)(nil),
				(*depstore.Factory)(nil),
				(*io.Closer)(nil),
				(*storagev1.SamplingStoreFactory)(nil),
			},
		},
		{
			name: "Implements both Purger and SamplingStoreFactory",
			factory: struct {
				storagev1.Factory
				storagev1.Purger
				storagev1.SamplingStoreFactory
			}{mockFactory, mockPurger, mockSamplingStoreFactory},
			expectedInterfaces: []any{
				(*tracestore.Factory)(nil),
				(*depstore.Factory)(nil),
				(*io.Closer)(nil),
				(*storagev1.Purger)(nil),
				(*storagev1.SamplingStoreFactory)(nil),
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

func TestAdapterCloseNotOk(t *testing.T) {
	f := NewFactory(&factorymocks.Factory{})
	closer, ok := f.(io.Closer)
	require.True(t, ok)
	require.NoError(t, closer.Close())
}

func TestAdapterClose(t *testing.T) {
	f := NewFactory(grpc.NewFactory())
	closer, ok := f.(io.Closer)
	require.True(t, ok)
	require.NoError(t, closer.Close())
}

func TestAdapterCreateTraceReader(t *testing.T) {
	f1 := new(factorymocks.Factory)
	f1.On("CreateSpanReader").Return(new(spanstoremocks.Reader), nil)

	f := NewFactory(f1)
	_, err := f.CreateTraceReader()
	require.NoError(t, err)
}

func TestAdapterCreateTraceReaderError(t *testing.T) {
	f1 := new(factorymocks.Factory)
	f1.On("CreateSpanReader").Return(nil, errors.New("mock error"))

	f := NewFactory(f1)
	_, err := f.CreateTraceReader()
	require.ErrorContains(t, err, "mock error")
}

func TestAdapterCreateTraceWriterError(t *testing.T) {
	f1 := new(factorymocks.Factory)
	f1.On("CreateSpanWriter").Return(nil, errors.New("mock error"))

	f := NewFactory(f1)
	_, err := f.CreateTraceWriter()
	require.ErrorContains(t, err, "mock error")
}

func TestAdapterCreateTraceWriter(t *testing.T) {
	f1 := new(factorymocks.Factory)
	f1.On("CreateSpanWriter").Return(new(spanstoremocks.Writer), nil)

	f := NewFactory(f1)
	_, err := f.CreateTraceWriter()
	require.NoError(t, err)
}

func TestAdapterCreateDependencyReader(t *testing.T) {
	f1 := new(factorymocks.Factory)
	f1.On("CreateDependencyReader").Return(new(dependencystoremocks.Reader), nil)

	f := NewFactory(f1)
	depFactory, ok := f.(depstore.Factory)
	require.True(t, ok)
	r, err := depFactory.CreateDependencyReader()
	require.NoError(t, err)
	require.NotNil(t, r)
}

func TestAdapterCreateDependencyReaderError(t *testing.T) {
	f1 := new(factorymocks.Factory)
	testErr := errors.New("test error")
	f1.On("CreateDependencyReader").Return(nil, testErr)

	f := NewFactory(f1)
	depFactory, ok := f.(depstore.Factory)
	require.True(t, ok)
	r, err := depFactory.CreateDependencyReader()
	require.ErrorIs(t, err, testErr)
	require.Nil(t, r)
}
