// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package spanstore

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaegertracing/jaeger/internal/storage/v1/cassandra/spanstore/dbmodel"
)

func TestWriterOptions(t *testing.T) {
	opts := applyOptions(TagFilter(dbmodel.DefaultTagFilter), IndexFilter(dbmodel.DefaultIndexFilter))
	assert.Equal(t, dbmodel.DefaultTagFilter, opts.tagFilter)
	assert.ObjectsAreEqual(dbmodel.DefaultIndexFilter, opts.indexFilter)
}

func TestWriterOptions_StorageMode(t *testing.T) {
	tests := []struct {
		name     string
		expected storageMode
		opts     Options
	}{
		{
			name:     "Default",
			expected: indexFlag | storeFlag,
			opts:     applyOptions(),
		},
		{
			name:     "Index Only",
			expected: indexFlag,
			opts:     applyOptions(StoreIndexesOnly()),
		},
		{
			name:     "Store Only",
			expected: storeFlag,
			opts:     applyOptions(StoreWithoutIndexing()),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.opts.storageMode)
		})
	}
}
