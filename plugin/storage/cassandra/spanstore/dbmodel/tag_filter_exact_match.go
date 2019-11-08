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

package dbmodel

import (
	"github.com/jaegertracing/jaeger/model"
)

// ExactMatchTagFilter filters out all tags in its tags slice
type ExactMatchTagFilter struct {
	tags        map[string]struct{}
	dropMatches bool
}

// NewExactMatchTagFilter creates a ExactMatchTagFilter with the provided tags.  Passing
// dropMatches true will exhibit blacklist behavior.  Passing dropMatches false
// will exhibit whitelist behavior.
func NewExactMatchTagFilter(tags []string, dropMatches bool) ExactMatchTagFilter {
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
	return NewExactMatchTagFilter(tags, true)
}

// NewWhitelistFilter is a convenience method for creating a whitelist ExactMatchTagFilter
func NewWhitelistFilter(tags []string) ExactMatchTagFilter {
	return NewExactMatchTagFilter(tags, false)
}

// FilterProcessTags implements TagFilter
func (tf ExactMatchTagFilter) FilterProcessTags(span *model.Span, processTags model.KeyValues) model.KeyValues {
	return tf.filter(processTags)
}

// FilterTags implements TagFilter
func (tf ExactMatchTagFilter) FilterTags(span *model.Span, tags model.KeyValues) model.KeyValues {
	return tf.filter(tags)
}

// FilterLogFields implements TagFilter
func (tf ExactMatchTagFilter) FilterLogFields(span *model.Span, logFields model.KeyValues) model.KeyValues {
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
