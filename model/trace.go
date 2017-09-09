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

package model

// Trace is a directed acyclic graph of Spans
type Trace struct {
	Spans    []*Span  `json:"spans,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
}

// FindSpanByID looks for a span with given span ID and returns the first one
// it finds (search order is unspecified), or nil if no spans have that ID.
func (t *Trace) FindSpanByID(id SpanID) *Span {
	for _, span := range t.Spans {
		if id == span.SpanID {
			return span
		}
	}
	return nil
}

// NormalizeTimestamps changes all timestamps in this trace to UTC.
func (t *Trace) NormalizeTimestamps() {
	for _, span := range t.Spans {
		span.NormalizeTimestamps()
	}
}
