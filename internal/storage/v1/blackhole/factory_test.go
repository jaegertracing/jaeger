// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package blackhole

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/metrics/api"
	"github.com/jaegertracing/jaeger/internal/storage/v1"
)

var _ storage.Factory = new(Factory)

func TestStorageFactory(t *testing.T) {
	f := NewFactory()
	require.NoError(t, f.Initialize(api.NullFactory, zap.NewNop()))
	assert.NotNil(t, f.store)
	reader, err := f.CreateSpanReader()
	require.NoError(t, err)
	assert.Equal(t, f.store, reader)
	writer, err := f.CreateSpanWriter()
	require.NoError(t, err)
	assert.Equal(t, f.store, writer)
	depReader, err := f.CreateDependencyReader()
	require.NoError(t, err)
	assert.Equal(t, f.store, depReader)
}
