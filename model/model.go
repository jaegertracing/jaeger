// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package model

import (
	jaegerIdlModel "github.com/jaegertracing/jaeger-idl/model/v1"
)

// This is a compile-time assertion to ensure that this generated file
// is compatible with the proto package it is being compiled against.
// A compilation error at this line likely means your copy of the
// proto package needs to be updated.

type ValueType = jaegerIdlModel.ValueType

const (
	ValueType_STRING  ValueType = jaegerIdlModel.ValueType_STRING
	ValueType_BOOL    ValueType = jaegerIdlModel.ValueType_BOOL
	ValueType_INT64   ValueType = jaegerIdlModel.ValueType_INT64
	ValueType_FLOAT64 ValueType = jaegerIdlModel.ValueType_FLOAT64
	ValueType_BINARY  ValueType = jaegerIdlModel.ValueType_BINARY
)

var ValueType_name = jaegerIdlModel.ValueType_name

var ValueType_value = jaegerIdlModel.ValueType_value

type SpanRefType = jaegerIdlModel.SpanRefType

const (
	SpanRefType_CHILD_OF     SpanRefType = jaegerIdlModel.SpanRefType_CHILD_OF
	SpanRefType_FOLLOWS_FROM SpanRefType = jaegerIdlModel.SpanRefType_FOLLOWS_FROM
)

var SpanRefType_name = jaegerIdlModel.SpanRefType_name

var SpanRefType_value = jaegerIdlModel.SpanRefType_value

type KeyValue = jaegerIdlModel.KeyValue

type Log = jaegerIdlModel.Log

type SpanRef = jaegerIdlModel.SpanRef

type Process = jaegerIdlModel.Process

type Span = jaegerIdlModel.Span

type Trace = jaegerIdlModel.Trace

type Trace_ProcessMapping = jaegerIdlModel.Trace_ProcessMapping

// Note that both Span and Batch may contain a Process.
// This is different from the Thrift model which was only used
// for transport, because Proto model is also used by the backend
// as the domain model, where once a batch is received it is split
// into individual spans which are all processed independently,
// and therefore they all need a Process. As far as on-the-wire
// semantics, both Batch and Spans in the same message may contain
// their own instances of Process, with span.Process taking priority
// over batch.Process.
type Batch = jaegerIdlModel.Batch

type DependencyLink = jaegerIdlModel.DependencyLink

var (
	ErrInvalidLengthModel        = jaegerIdlModel.ErrInvalidLengthModel
	ErrIntOverflowModel          = jaegerIdlModel.ErrIntOverflowModel
	ErrUnexpectedEndOfGroupModel = jaegerIdlModel.ErrUnexpectedEndOfGroupModel
)
