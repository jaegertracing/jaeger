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

package dbmodel

import (
	"github.com/jaegertracing/jaeger/model"
)

// TagFilter filters out any tags that should not be indexed.
type TagFilter interface {
	FilterProcessTags(span *model.Span, processTags model.KeyValues) model.KeyValues
	FilterTags(span *model.Span, tags model.KeyValues) model.KeyValues
	FilterLogFields(span *model.Span, logFields model.KeyValues) model.KeyValues
}

// NewCompositeFilter returns a TagFilter that implements all of the passed filters in order.
func NewCompositeFilter(filters ...TagFilter) TagFilter {
	if len(filters) == 1 {
		return filters[0]
	}

	return NewChainedTagFilter(filters...)
}

// ChainedTagFilter applies multiple tag filters in serial fashion.
type ChainedTagFilter []TagFilter

// NewChainedTagFilter creates a TagFilter from the variadic list of passed TagFilter.
func NewChainedTagFilter(filters ...TagFilter) ChainedTagFilter {
	return filters
}

// FilterProcessTags calls each FilterProcessTags.
func (tf ChainedTagFilter) FilterProcessTags(span *model.Span, processTags model.KeyValues) model.KeyValues {
	for _, f := range tf {
		processTags = f.FilterProcessTags(span, processTags)
	}
	return processTags
}

// FilterTags calls each FilterTags
func (tf ChainedTagFilter) FilterTags(span *model.Span, tags model.KeyValues) model.KeyValues {
	for _, f := range tf {
		tags = f.FilterTags(span, tags)
	}
	return tags
}

// FilterLogFields calls each FilterLogFields
func (tf ChainedTagFilter) FilterLogFields(span *model.Span, logFields model.KeyValues) model.KeyValues {
	for _, f := range tf {
		logFields = f.FilterProcessTags(span, logFields)
	}
	return logFields
}

// DefaultTagFilter returns a filter that retrieves all tags from span.Tags, span.Logs, and span.Process.
var DefaultTagFilter = tagFilterImpl{}

type tagFilterImpl struct{}

func (f tagFilterImpl) FilterProcessTags(span *model.Span, processTags model.KeyValues) model.KeyValues {
	return processTags
}

func (f tagFilterImpl) FilterTags(span *model.Span, tags model.KeyValues) model.KeyValues {
	return tags
}

func (f tagFilterImpl) FilterLogFields(span *model.Span, logFields model.KeyValues) model.KeyValues {
	return logFields
}
