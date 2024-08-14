// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
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
	reader, err := f.CreateSpanReader()
	require.NoError(t, err)
	assert.Equal(t, f.store, reader)
	writer, err := f.CreateSpanWriter()
	require.NoError(t, err)
	assert.Equal(t, f.store, writer)
	reader, err = f.CreateArchiveSpanReader()
	require.NoError(t, err)
	assert.Equal(t, f.store, reader)
	writer, err = f.CreateArchiveSpanWriter()
	require.NoError(t, err)
	assert.Equal(t, f.store, writer)
	depReader, err := f.CreateDependencyReader()
	require.NoError(t, err)
	assert.Equal(t, f.store, depReader)
}
