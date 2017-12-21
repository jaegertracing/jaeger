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
	"testing"

	"github.com/kr/pretty"
	"github.com/stretchr/testify/assert"

	"github.com/jaegertracing/jaeger/model"
)

func TestDefaultTagFilter(t *testing.T) {
	span := getTestJaegerSpan()
	expectedTags := append(append(someTags, someTags...), someTags...)
	filteredTags := DefaultTagFilter.FilterProcessTags(span.Process.Tags)
	filteredTags = append(filteredTags, DefaultTagFilter.FilterTags(span.Tags)...)
	for _, log := range span.Logs {
		filteredTags = append(filteredTags, DefaultTagFilter.FilterLogFields(log.Fields)...)
	}
	if !assert.EqualValues(t, expectedTags, filteredTags) {
		for _, diff := range pretty.Diff(expectedTags, filteredTags) {
			t.Log(diff)
		}
	}
}

type onlyStringsFilter struct{}

func (f onlyStringsFilter) filterStringTags(tags model.KeyValues) model.KeyValues {
	var ret model.KeyValues
	for _, tag := range tags {
		if tag.VType == model.StringType {
			ret = append(ret, tag)
		}
	}
	return ret
}

func (f onlyStringsFilter) FilterProcessTags(processTags model.KeyValues) model.KeyValues {
	return f.filterStringTags(processTags)
}

func (f onlyStringsFilter) FilterTags(tags model.KeyValues) model.KeyValues {
	return f.filterStringTags(tags)
}

func (f onlyStringsFilter) FilterLogFields(logFields model.KeyValues) model.KeyValues {
	return f.filterStringTags(logFields)
}

func TestChainedTagFilter(t *testing.T) {
	tags := someTags
	expectedTags := model.KeyValues{model.String(someStringTagKey, someStringTagValue)}
	filter := NewChainedTagFilter(DefaultTagFilter, onlyStringsFilter{})
	filteredTags := filter.FilterProcessTags(tags)
	if !assert.EqualValues(t, expectedTags, filteredTags) {
		for _, diff := range pretty.Diff(expectedTags, filteredTags) {
			t.Log(diff)
		}
	}
}
