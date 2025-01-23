// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package model

import (
	jaegerIdlModel "github.com/jaegertracing/jaeger-idl/model/v1"
)

// SortTraceIDs sorts a list of TraceIDs
func SortTraceIDs(traceIDs []*TraceID) {
	jaegerIdlModel.SortTraceIDs(traceIDs)
}

// SortTraces deep sorts a list of traces by TraceID.
func SortTraces(traces []*Trace) {
	jaegerIdlModel.SortTraces(traces)
}

// SortTrace deep sorts a trace's spans by SpanID.
func SortTrace(trace *Trace) {
	jaegerIdlModel.SortTrace(trace)
}

// SortSpan deep sorts a span: this sorts its tags, logs by timestamp, tags in logs, and tags in process.
func SortSpan(span *Span) {
	jaegerIdlModel.SortSpan(span)
}
