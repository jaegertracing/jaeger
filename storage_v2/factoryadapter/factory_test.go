// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package factoryadapter

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/plugin/storage/grpc"
	factoryMocks "github.com/jaegertracing/jaeger/storage/mocks"
	spanstoreMocks "github.com/jaegertracing/jaeger/storage/spanstore/mocks"
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
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("create trace reader did not panic")
		}
	}()

	f := &Factory{}
	f.CreateTraceReader()
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
