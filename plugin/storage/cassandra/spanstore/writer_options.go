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
	"github.com/jaegertracing/jaeger/plugin/storage/cassandra/spanstore/dbmodel"
)

// Option is a function that sets some option on the writer.
type Option func(c *Options)

// Options control behavior of the writer.
type Options struct {
	tagFilter   dbmodel.TagFilter
	storageMode storageMode
}

// TagFilter can be provided to filter any tags that should not be indexed.
func TagFilter(tagFilter dbmodel.TagFilter) Option {
	return func(o *Options) {
		o.tagFilter = tagFilter
	}
}

// StoreIndexesOnly can be provided to skip storing spans, and only store span indexes.
func StoreIndexesOnly() Option {
	return func(o *Options) {
		o.storageMode = indexOnly
	}
}

// StoreWithoutIndexing can be provided to store spans without indexing them.
func StoreWithoutIndexing() Option {
	return func(o *Options) {
		o.storageMode = storeOnly
	}
}

func applyOptions(opts ...Option) Options {
	o := Options{}
	for _, opt := range opts {
		opt(&o)
	}
	if o.tagFilter == nil {
		o.tagFilter = dbmodel.DefaultTagFilter
	}
	return o
}
