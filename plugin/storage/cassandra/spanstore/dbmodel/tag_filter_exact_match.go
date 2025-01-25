// Copyright (c) 2019 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package dbmodel

import (
	"github.com/jaegertracing/jaeger-idl/model/v1"
)

// ExactMatchTagFilter filters out all tags in its tags slice
type ExactMatchTagFilter struct {
	tags        map[string]struct{}
	dropMatches bool
}

// newExactMatchTagFilter creates a ExactMatchTagFilter with the provided tags.  Passing
// dropMatches true will exhibit blacklist behavior.  Passing dropMatches false
// will exhibit whitelist behavior.
func newExactMatchTagFilter(tags []string, dropMatches bool) ExactMatchTagFilter {
	mapTags := make(map[string]struct{})
	for _, t := range tags {
		mapTags[t] = struct{}{}
	}
	return ExactMatchTagFilter{
		tags:        mapTags,
		dropMatches: dropMatches,
	}
}

// NewBlacklistFilter is a convenience method for creating a blacklist ExactMatchTagFilter
func NewBlacklistFilter(tags []string) ExactMatchTagFilter {
	return newExactMatchTagFilter(tags, true)
}

// NewWhitelistFilter is a convenience method for creating a whitelist ExactMatchTagFilter
func NewWhitelistFilter(tags []string) ExactMatchTagFilter {
	return newExactMatchTagFilter(tags, false)
}

// FilterProcessTags implements TagFilter
func (tf ExactMatchTagFilter) FilterProcessTags(_ *model.Span, processTags model.KeyValues) model.KeyValues {
	return tf.filter(processTags)
}

// FilterTags implements TagFilter
func (tf ExactMatchTagFilter) FilterTags(_ *model.Span, tags model.KeyValues) model.KeyValues {
	return tf.filter(tags)
}

// FilterLogFields implements TagFilter
func (tf ExactMatchTagFilter) FilterLogFields(_ *model.Span, logFields model.KeyValues) model.KeyValues {
	return tf.filter(logFields)
}

func (tf ExactMatchTagFilter) filter(tags model.KeyValues) model.KeyValues {
	var filteredTags model.KeyValues
	for _, t := range tags {
		if _, ok := tf.tags[t.Key]; ok == !tf.dropMatches {
			filteredTags = append(filteredTags, t)
		}
	}
	return filteredTags
}
