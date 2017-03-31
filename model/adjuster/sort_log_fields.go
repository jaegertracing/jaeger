// Copyright (c) 2017 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package adjuster

import (
	"github.com/uber/jaeger/model"
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
