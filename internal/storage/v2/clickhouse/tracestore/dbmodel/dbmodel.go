// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package dbmodel

import "time"

// Trace Domain model in Clickhouse.
// This struct represents the schema for storing OTel pipeline Traces in Clickhouse.
type Trace struct {
	Resource Resource
	Scope    Scope
	Span     Span
	Links    []Link
	Events   []Event
}

type Resource struct {
	Attributes AttributesGroup
}

type Scope struct {
	Name       string
	Version    string
	Attributes AttributesGroup
}

type Span struct {
	Timestamp     time.Time
	TraceId       []byte
	SpanId        []byte
	ParentSpanId  []byte
	TraceState    string
	Name          string
	Kind          string
	Duration      time.Time
	StatusCode    string
	StatusMessage string
	Attributes    AttributesGroup
}

type Link struct {
	TraceId    []byte
	SpanId     []byte
	TraceState string
	Attributes AttributesGroup
}

type Event struct {
	Name       string
	Timestamp  time.Time
	Attributes AttributesGroup
}

// AttributesGroup captures all data from a single pcommon.Map, except
// complex attributes (like slice or map) which are currently not supported.
// AttributesGroup consists of pairs of vectors for each of the supported primitive
// types, e.g. (BoolKeys, BoolValues). Every attribute in the pcommon.Map is mapped
// to one of the pairs depending on its type. The slices in each pair have identical
// length, which may be different from length in another pair. For example, if the
// pcommon.Map has no Boolean attributes then (BoolKeys=[], BoolValues=[]).
type AttributesGroup struct {
	BoolKeys     []string
	BoolValues   []bool
	DoubleKeys   []string
	DoubleValues []float64
	IntKeys      []string
	IntValues    []int64
	StrKeys      []string
	StrValues    []string
	BytesKeys    []string
	BytesValues  [][]byte
}
