// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package storage

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/metricstest"
	depstoremocks "github.com/jaegertracing/jaeger/storage/dependencystore/mocks"
	"github.com/jaegertracing/jaeger/storage/mocks"
	spanstoremocks "github.com/jaegertracing/jaeger/storage/spanstore/mocks"
)

func TestInitialize_DelegatesUnderlyingResponse(t *testing.T) {
	mf := metricstest.NewFactory(0)
	f := mocks.Factory{}
	logger := zap.NewNop()
	df := NewDecoratorFactory(&f, mf)
	expectedErr := errors.New("test error")
	f.On("Initialize", mf, logger).Return(expectedErr)
	err := df.Initialize(mf, logger)
	require.ErrorIs(t, err, expectedErr)
}

func TestCreateSpanReader_DelegatesErrorResponse(t *testing.T) {
	mf := metricstest.NewFactory(0)
	f := mocks.Factory{}
	expectedReader := &spanstoremocks.Reader{}
	expectedErr := errors.New("test error")
	df := NewDecoratorFactory(&f, mf)
	f.On("CreateSpanReader").Return(expectedReader, expectedErr)
	r, err := df.CreateSpanReader()
	require.ErrorIs(t, err, expectedErr)
	require.Equal(t, expectedReader, r)

	counters, gauges := mf.Snapshot()
	require.Empty(t, counters)
	require.Empty(t, gauges)
}

func TestCreateSpanReader_ReturnsDecoratedReader(t *testing.T) {
	mf := metricstest.NewFactory(0)
	f := mocks.Factory{}
	expectedReader := &spanstoremocks.Reader{}
	df := NewDecoratorFactory(&f, mf)
	f.On("CreateSpanReader").Return(expectedReader, nil)
	r, err := df.CreateSpanReader()
	require.NoError(t, err)

	// make a request with the decorated reader to ensure that a metric gets recorded
	expectedReader.On("GetServices", context.Background()).Return([]string{}, nil)
	r.GetServices(context.Background())
	counters, _ := mf.Snapshot()
	require.EqualValues(t, map[string]int64{"requests|operation=get_services|result=ok": 1}, counters)
}

func TestCreateSpanWriter_DelegatesUnderlyingResponse(t *testing.T) {
	mf := metricstest.NewFactory(0)
	f := mocks.Factory{}
	expectedWriter := &spanstoremocks.Writer{}
	expectedErr := errors.New("test error")
	df := NewDecoratorFactory(&f, mf)
	f.On("CreateSpanWriter").Return(expectedWriter, expectedErr)
	w, err := df.CreateSpanWriter()
	require.ErrorIs(t, err, expectedErr)
	require.Equal(t, expectedWriter, w)
}

func TestCreateDependencyReader_DelegatesUnderlyingResponse(t *testing.T) {
	mf := metricstest.NewFactory(0)
	f := mocks.Factory{}
	expectedReader := &depstoremocks.Reader{}
	expectedErr := errors.New("test error")
	df := NewDecoratorFactory(&f, mf)
	f.On("CreateDependencyReader").Return(expectedReader, expectedErr)
	r, err := df.CreateDependencyReader()
	require.ErrorIs(t, err, expectedErr)
	require.Equal(t, expectedReader, r)
}
