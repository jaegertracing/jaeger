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

package adjuster

import (
	"github.com/jaegertracing/jaeger/model"
)

// SortLogFields returns an Adjuster that sorts the fields in the span logs.
// It puts the `event` field in the first position (if present), and sorts
// all other fields lexicographically.
//
// TODO: it should also do something about the "msg" field, maybe replace it
// with "event" field.
// TODO: we may also want to move "level" field (as in logging level) to an earlier
// place in the list. This adjuster needs some sort of config describing predefined
// field names/types and their relative order.
func SortLogFields() Adjuster {
	return Func(func(trace *model.Trace) (*model.Trace, error) {
		for _, span := range trace.Spans {
			for _, log := range span.Logs {
				// first move "event" field into the first position
				offset := 0
				for i, field := range log.Fields {
					if field.Key == "event" && field.VType == model.StringType {
						if i > 0 {
							log.Fields[0], log.Fields[i] = log.Fields[i], log.Fields[0]
						}
						offset = 1
						break
					}
				}
				// sort all remaining fields
				if len(log.Fields) > 1 {
					model.KeyValues(log.Fields[offset:]).Sort()
				}
			}
		}
		return trace, nil
	})
}
