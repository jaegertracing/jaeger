// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package blackhole

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/pkg/metrics"
	"github.com/jaegertracing/jaeger/storage"
)

var _ storage.Factory = new(Factory)

func TestStorageFactory(t *testing.T) {
	f := NewFactory()
	require.NoError(t, f.Initialize(metrics.NullFactory, zap.NewNop()))
	assert.NotNil(t, f.store)
	_, err := f.CreateSpanReader()
	require.NoError(t, err)
	writer, err := f.CreateSpanWriter()
	require.NoError(t, err)
	assert.Equal(t, f.store, writer)
	_, err = f.CreateArchiveSpanReader()
	require.NoError(t, err)
	writer, err = f.CreateArchiveSpanWriter()
	require.NoError(t, err)
	assert.Equal(t, f.store, writer)
	depReader, err := f.CreateDependencyReader()
	require.NoError(t, err)
	assert.Equal(t, f.store, depReader)
}
