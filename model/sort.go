// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package model

import (
	modelv1 "github.com/jaegertracing/jaeger-idl/model/v1"
)

// SortTraceIDs sorts a list of TraceIDs
var SortTraceIDs = modelv1.SortTraceIDs

// SortTraces deep sorts a list of traces by TraceID.
var SortTraces = modelv1.SortTraces

// SortTrace deep sorts a trace's spans by SpanID.
var SortTrace = modelv1.SortTrace

// SortSpan deep sorts a span: this sorts its tags, logs by timestamp, tags in logs, and tags in process.
var SortSpan = modelv1.SortSpan
