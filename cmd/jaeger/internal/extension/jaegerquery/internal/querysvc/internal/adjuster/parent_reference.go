// Copyright (c) 2022 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package adjuster

import (
	"github.com/jaegertracing/jaeger-idl/model/v1"
)

// ParentReference returns an Adjuster that puts CHILD_OF references first.
// This is necessary to match jaeger-ui expectations:
// * https://github.com/jaegertracing/jaeger-ui/issues/966
func ParentReference() Adjuster {
	return Func(func(trace *model.Trace) {
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
	})
}
