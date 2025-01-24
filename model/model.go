// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package model

import (
	modelv1 "github.com/jaegertracing/jaeger-idl/model/v1"
)

// This is a compile-time assertion to ensure that this generated file
// is compatible with the proto package it is being compiled against.
// A compilation error at this line likely means your copy of the
// proto package needs to be updated.

type ValueType = modelv1.ValueType

const (
	ValueType_STRING  ValueType = modelv1.ValueType_STRING
	ValueType_BOOL    ValueType = modelv1.ValueType_BOOL
	ValueType_INT64   ValueType = modelv1.ValueType_INT64
	ValueType_FLOAT64 ValueType = modelv1.ValueType_FLOAT64
	ValueType_BINARY  ValueType = modelv1.ValueType_BINARY
)

type SpanRefType = modelv1.SpanRefType

const (
	SpanRefType_CHILD_OF     SpanRefType = modelv1.SpanRefType_CHILD_OF
	SpanRefType_FOLLOWS_FROM SpanRefType = modelv1.SpanRefType_FOLLOWS_FROM
)

type KeyValue = modelv1.KeyValue

type Log = modelv1.Log

type SpanRef = modelv1.SpanRef

type Process = modelv1.Process

type Span = modelv1.Span

type Trace = modelv1.Trace

type Trace_ProcessMapping = modelv1.Trace_ProcessMapping

// Note that both Span and Batch may contain a Process.
// This is different from the Thrift model which was only used
// for transport, because Proto model is also used by the backend
// as the domain model, where once a batch is received it is split
// into individual spans which are all processed independently,
// and therefore they all need a Process. As far as on-the-wire
// semantics, both Batch and Spans in the same message may contain
// their own instances of Process, with span.Process taking priority
// over batch.Process.
type Batch = modelv1.Batch

type DependencyLink = modelv1.DependencyLink
