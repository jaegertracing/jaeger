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
			if len(span.References) == 0 {
				continue
			}

			for i := range span.References {
				if i == 0 {
					continue
				}

				if span.References[0].RefType == model.SpanRefType_CHILD_OF && span.References[0].TraceID == span.TraceID {
					break
				}

				if span.References[0].TraceID != span.TraceID {
					if span.References[i].TraceID == span.TraceID {
						span.References[i], span.References[0] = span.References[0], span.References[i]
					}
				} else if span.References[0].RefType != model.ChildOf && span.References[i].RefType == model.ChildOf {
					span.References[i], span.References[0] = span.References[0], span.References[i]
				}
			}
		}

		return trace, nil
	})
}
