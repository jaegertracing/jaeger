// Copyright (c) 2022 The Jaeger Authors.
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

// ParentReference returns an Adjuster that puts CHILD_OF references first.
// This is necessary to match jaeger-ui expectations:
// * https://github.com/jaegertracing/jaeger-ui/issues/966
func ParentReference() Adjuster {
	return Func(func(trace *model.Trace) (*model.Trace, error) {
		for _, span := range trace.Spans {
			firstChildOfRef := -1
			firstOtherRef := -1
			for i := 0; i < len(span.References); i++ {
				if span.References[i].TraceID == span.TraceID {
					if span.References[i].RefType == model.SpanRefType_CHILD_OF {
						firstChildOfRef = i
						break
					} else if firstOtherRef == -1 {
						firstOtherRef = i
					}
				}
			}
			swap := firstChildOfRef
			if swap == -1 {
				swap = firstOtherRef
			}
			if swap != 0 && swap != -1 {
				span.References[swap], span.References[0] = span.References[0], span.References[swap]
			}
		}

		return trace, nil
	})
}
