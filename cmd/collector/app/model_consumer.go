// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"github.com/jaegertracing/jaeger/cmd/collector/app/processor"
	"github.com/jaegertracing/jaeger/model"
)

// ProcessSpan processes a Domain Model Span
type ProcessSpan func(span *model.Span, tenant string)

// ProcessSpans processes a batch of Domain Model Spans
type ProcessSpans func(spans processor.Batch)

// FilterSpan decides whether to allow or disallow a span
type FilterSpan func(span *model.Span) bool

// ChainedProcessSpan chains spanProcessors as a single ProcessSpan call
func ChainedProcessSpan(spanProcessors ...ProcessSpan) ProcessSpan {
	return func(span *model.Span, tenant string) {
		for _, processor := range spanProcessors {
			processor(span, tenant)
		}
	}
}
