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

package spanstore

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaegertracing/jaeger/plugin/storage/cassandra/spanstore/dbmodel"
)

func TestWriterOptions(t *testing.T) {
	opts := applyOptions(TagFilter(dbmodel.DefaultTagFilter))
	assert.Equal(t, dbmodel.DefaultTagFilter, opts.tagFilter)
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
