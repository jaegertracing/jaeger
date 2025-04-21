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
	TraceId       string
	SpanId        string
	ParentSpanId  string
	TraceState    string
	Name          string
	Kind          string
	Duration      time.Time
	StatusCode    string
	StatusMessage string
	Attributes    AttributesGroup
}

type Link struct {
	TraceId    string
	SpanId     string
	TraceState string
	Attributes AttributesGroup
}

type Event struct {
	Name       string
	Timestamp  time.Time
	Attributes AttributesGroup
}
