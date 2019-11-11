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

// TagFilterDropAll filters all fields of a given type.
type TagFilterDropAll struct {
	dropTags        bool
	dropProcessTags bool
	dropLogs        bool
}

// NewTagFilterDropAll return a filter that filters all of the specified type
func NewTagFilterDropAll(dropTags bool, dropProcessTags bool, dropLogs bool) TagFilterDropAll {
	return TagFilterDropAll{
		dropTags:        dropTags,
		dropProcessTags: dropProcessTags,
		dropLogs:        dropLogs,
	}
}

// FilterProcessTags implements TagFilter
func (f TagFilterDropAll) FilterProcessTags(span *model.Span, processTags model.KeyValues) model.KeyValues {
	if f.dropProcessTags {
		return model.KeyValues{}
	}
	return processTags
}

// FilterTags implements TagFilter
func (f TagFilterDropAll) FilterTags(span *model.Span, tags model.KeyValues) model.KeyValues {
	if f.dropTags {
		return model.KeyValues{}
	}
	return tags
}

// FilterLogFields implements TagFilter
func (f TagFilterDropAll) FilterLogFields(span *model.Span, logFields model.KeyValues) model.KeyValues {
	if f.dropLogs {
		return model.KeyValues{}
	}
	return logFields
}
