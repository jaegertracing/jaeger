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

// WhitelistTagFilter filters out all tags in its tags slice
type WhitelistTagFilter struct {
	tags map[string]struct{}
}

// NewWhitelistTagFilter creates a WhitelistTagFilter with the provided tags
func NewWhitelistTagFilter(tags []string) WhitelistTagFilter {
	mapTags := make(map[string]struct{})
	for _, t := range tags {
		mapTags[t] = struct{}{}
	}
	return WhitelistTagFilter{
		tags: mapTags,
	}
}

// FilterProcessTags implements TagFilter
func (tf WhitelistTagFilter) FilterProcessTags(span *model.Span, tags model.KeyValues) model.KeyValues {
	return tf.filter(tags)
}

// FilterTags implements TagFilter
func (tf WhitelistTagFilter) FilterTags(span *model.Span, tags model.KeyValues) model.KeyValues {
	return tf.filter(tags)
}

// FilterLogFields implements TagFilter
func (tf WhitelistTagFilter) FilterLogFields(span *model.Span, logFields model.KeyValues) model.KeyValues {
	return tf.filter(logFields)
}

func (tf WhitelistTagFilter) filter(tags model.KeyValues) model.KeyValues {
	var kvs model.KeyValues
	for _, t := range tags {
		if _, ok := tf.tags[t.Key]; ok {
			kvs = append(kvs, t)
		}
	}
	return kvs
}
