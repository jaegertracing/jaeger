// Copyright (c) 2020 The Jaeger Authors.
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
	"github.com/jaegertracing/jaeger/plugin/storage/grpc/shared"
	"github.com/jaegertracing/jaeger/plugin/storage/memory"
	"github.com/jaegertracing/jaeger/storage/dependencystore"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

var (
	_ shared.StoragePlugin             = (*storagePlugin)(nil)
	_ shared.ArchiveStoragePlugin      = (*storagePlugin)(nil)
	_ shared.StreamingSpanWriterPlugin = (*storagePlugin)(nil)
)

type storagePlugin struct {
	store        *memory.Store
	archiveStore *memory.Store
}

func NewStoragePlugin(mainStore *memory.Store, archiveStore *memory.Store) *storagePlugin {
	return &storagePlugin{store: mainStore, archiveStore: archiveStore}
}

func (ns *storagePlugin) DependencyReader() dependencystore.Reader {
	return ns.store
}

func (ns *storagePlugin) SpanReader() spanstore.Reader {
	return ns.store
}

func (ns *storagePlugin) SpanWriter() spanstore.Writer {
	return ns.store
}

func (ns *storagePlugin) StreamingSpanWriter() spanstore.Writer {
	return ns.store
}

func (ns *storagePlugin) ArchiveSpanReader() spanstore.Reader {
	return ns.archiveStore
}

func (ns *storagePlugin) ArchiveSpanWriter() spanstore.Writer {
	return ns.archiveStore
}
