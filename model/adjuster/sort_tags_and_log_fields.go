// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package adjuster

import (
	"github.com/jaegertracing/jaeger/model"
)

// SortTagsAndLogFields returns an Adjuster that sorts the fields in the tags and
// span logs.
//
// For span logs, it puts the `event` field in the first position (if present), and
// sorts all other fields lexicographically.
//
// TODO: it should also do something about the "msg" field, maybe replace it
// with "event" field.
// TODO: we may also want to move "level" field (as in logging level) to an earlier
// place in the list. This adjuster needs some sort of config describing predefined
// field names/types and their relative order.
func SortTagsAndLogFields() Adjuster {
	return Func(func(trace *model.Trace) (*model.Trace, error) {
		for _, span := range trace.Spans {
			model.KeyValues(span.Tags).Sort()
			if span.Process != nil {
				model.KeyValues(span.Process.Tags).Sort()
			}

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
