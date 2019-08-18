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

// LogFieldsFilter filters all span.Logs.Fields.
type LogFieldsFilter struct {
	tagFilterImpl
}

// NewLogFieldsFilter return a filter that filters all span.Logs.Fields.
func NewLogFieldsFilter() *LogFieldsFilter {
	return &LogFieldsFilter{}
}

// FilterLogFields implements TagFilter#FilterLogFields
func (f *LogFieldsFilter) FilterLogFields(span *model.Span, logFields model.KeyValues) model.KeyValues {
	return model.KeyValues{}
}
