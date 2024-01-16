// Copyright (c) 2019 The Jaeger Authors.
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

package memory

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaegertracing/jaeger/pkg/testutils"
	"github.com/jaegertracing/jaeger/plugin/storage/memory"
)

func TestPluginUsesMemoryStorage(t *testing.T) {
	mainStorage := memory.NewStore()
	archiveStorage := memory.NewStore()

	memStorePlugin := NewStoragePlugin(mainStorage, archiveStorage)

	assert.Equal(t, mainStorage, memStorePlugin.DependencyReader())
	assert.Equal(t, mainStorage, memStorePlugin.SpanReader())
	assert.Equal(t, mainStorage, memStorePlugin.SpanWriter())
	assert.Equal(t, mainStorage, memStorePlugin.StreamingSpanWriter())
	assert.Equal(t, archiveStorage, memStorePlugin.ArchiveSpanReader())
	assert.Equal(t, archiveStorage, memStorePlugin.ArchiveSpanWriter())
}

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}
