// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package dbmodel

// TagFilter filters out any tags that should not be indexed.
type TagFilter interface {
	FilterProcessTags(span *Span, processTags []KeyValue) []KeyValue
	FilterTags(span *Span, tags []KeyValue) []KeyValue
	FilterLogFields(span *Span, logFields []KeyValue) []KeyValue
}

// ChainedTagFilter applies multiple tag filters in serial fashion.
type ChainedTagFilter []TagFilter

// NewChainedTagFilter creates a TagFilter from the variadic list of passed TagFilter.
func NewChainedTagFilter(filters ...TagFilter) ChainedTagFilter {
	return filters
}

// FilterProcessTags calls each FilterProcessTags.
func (tf ChainedTagFilter) FilterProcessTags(span *Span, processTags []KeyValue) []KeyValue {
	for _, f := range tf {
		processTags = f.FilterProcessTags(span, processTags)
	}
	return processTags
}

// FilterTags calls each FilterTags
func (tf ChainedTagFilter) FilterTags(span *Span, tags []KeyValue) []KeyValue {
	for _, f := range tf {
		tags = f.FilterTags(span, tags)
	}
	return tags
}

// FilterLogFields calls each FilterLogFields
func (tf ChainedTagFilter) FilterLogFields(span *Span, logFields []KeyValue) []KeyValue {
	for _, f := range tf {
		logFields = f.FilterLogFields(span, logFields)
	}
	return logFields
}

// DefaultTagFilter returns a filter that retrieves all tags from span.Tags, span.Logs, and span.Process.
var DefaultTagFilter = tagFilterImpl{}

type tagFilterImpl struct{}

func (tagFilterImpl) FilterProcessTags(_ *Span, processTags []KeyValue) []KeyValue {
	return processTags
}

func (tagFilterImpl) FilterTags(_ *Span, tags []KeyValue) []KeyValue {
	return tags
}

func (tagFilterImpl) FilterLogFields(_ *Span, logFields []KeyValue) []KeyValue {
	return logFields
}
