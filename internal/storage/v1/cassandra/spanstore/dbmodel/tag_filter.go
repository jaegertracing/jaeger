// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package dbmodel

import (
	"github.com/jaegertracing/jaeger-idl/model/v1"
)

// TagFilter filters out any tags that should not be indexed.
type TagFilter interface {
	FilterProcessTags(span *model.Span, processTags model.KeyValues) model.KeyValues
	FilterTags(span *model.Span, tags model.KeyValues) model.KeyValues
	FilterLogFields(span *model.Span, logFields model.KeyValues) model.KeyValues
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
		logFields = f.FilterLogFields(span, logFields)
	}
	return logFields
}

// DefaultTagFilter returns a filter that retrieves all tags from span.Tags, span.Logs, and span.Process.
var DefaultTagFilter = tagFilterImpl{}

type tagFilterImpl struct{}

func (tagFilterImpl) FilterProcessTags(_ *model.Span, processTags model.KeyValues) model.KeyValues {
	return processTags
}

func (tagFilterImpl) FilterTags(_ *model.Span, tags model.KeyValues) model.KeyValues {
	return tags
}

func (tagFilterImpl) FilterLogFields(_ *model.Span, logFields model.KeyValues) model.KeyValues {
	return logFields
}
