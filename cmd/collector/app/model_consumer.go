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

package app

import (
	"github.com/jaegertracing/jaeger/model"
)

// ProcessSpan processes a Domain Model Span
type ProcessSpan func(span *model.Span)

// ProcessSpans processes a batch of Domain Model Spans
type ProcessSpans func(spans []*model.Span)

// FilterSpan decides whether to allow or disallow a span
type FilterSpan func(span *model.Span) bool

// ChainedProcessSpan chains spanProcessors as a single ProcessSpan call
func ChainedProcessSpan(spanProcessors ...ProcessSpan) ProcessSpan {
	return func(span *model.Span) {
		for _, processor := range spanProcessors {
			processor(span)
		}
	}
}
