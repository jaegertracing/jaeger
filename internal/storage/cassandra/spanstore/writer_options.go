// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package spanstore

import (
	"github.com/jaegertracing/jaeger/internal/storage/cassandra/spanstore/dbmodel"
)

// Option is a function that sets some option on the writer.
type Option func(c *Options)

// Options control behavior of the writer.
type Options struct {
	tagFilter   dbmodel.TagFilter
	storageMode storageMode
	indexFilter dbmodel.IndexFilter
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
		o.storageMode = indexFlag
	}
}

// StoreWithoutIndexing can be provided to store spans without indexing them.
func StoreWithoutIndexing() Option {
	return func(o *Options) {
		o.storageMode = storeFlag
	}
}

// IndexFilter can be provided to filter certain spans that should not be indexed.
func IndexFilter(indexFilter dbmodel.IndexFilter) Option {
	return func(o *Options) {
		o.indexFilter = indexFilter
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
	if o.storageMode == 0 {
		o.storageMode = storeFlag | indexFlag
	}
	if o.indexFilter == nil {
		o.indexFilter = dbmodel.DefaultIndexFilter
	}
	return o
}
