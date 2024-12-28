// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2018 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package dbmodel

// ReferenceType is the reference type of one span to another
type ReferenceType string

// TraceID is the shared trace ID of all spans in the trace.
type TraceID string

// SpanID is the id of a span
type SpanID string

// ValueType is the type of a value stored in KeyValue struct.
type ValueType string

const (
	// ChildOf means a span is the child of another span
	ChildOf ReferenceType = "CHILD_OF"
	// FollowsFrom means a span follows from another span
	FollowsFrom ReferenceType = "FOLLOWS_FROM"

	// StringType indicates a string value stored in KeyValue
	StringType ValueType = "string"
	// BoolType indicates a Boolean value stored in KeyValue
	BoolType ValueType = "bool"
	// Int64Type indicates a 64bit signed integer value stored in KeyValue
	Int64Type ValueType = "int64"
	// Float64Type indicates a 64bit float value stored in KeyValue
	Float64Type ValueType = "float64"
	// BinaryType indicates an arbitrary byte array stored in KeyValue
	BinaryType ValueType = "binary"
)

// Span is ES database representation of the domain span.
type Span struct {
	TraceID       TraceID     `json:"traceID"`
	SpanID        SpanID      `json:"spanID"`
	ParentSpanID  SpanID      `json:"parentSpanID,omitempty"` // deprecated
	Flags         uint32      `json:"flags,omitempty"`
	OperationName string      `json:"operationName"`
	References    []Reference `json:"references"`
	StartTime     uint64      `json:"startTime"` // microseconds since Unix epoch
	// ElasticSearch does not support a UNIX Epoch timestamp in microseconds,
	// so Jaeger maps StartTime to a 'long' type. This extra StartTimeMillis field
	// works around this issue, enabling timerange queries.
	StartTimeMillis uint64     `json:"startTimeMillis"`
	Duration        uint64     `json:"duration"` // microseconds
	Tags            []KeyValue `json:"tags"`
	// Alternative representation of tags for better kibana support
	Tag     map[string]any `json:"tag,omitempty"`
	Logs    []Log          `json:"logs"`
	Process Process        `json:"process,omitempty"`
}

// Reference is a reference from one span to another
type Reference struct {
	RefType ReferenceType `json:"refType"`
	TraceID TraceID       `json:"traceID"`
	SpanID  SpanID        `json:"spanID"`
}

// Process is the process emitting a set of spans
type Process struct {
	ServiceName string     `json:"serviceName"`
	Tags        []KeyValue `json:"tags"`
	// Alternative representation of tags for better kibana support
	Tag map[string]any `json:"tag,omitempty"`
}

// Log is a log emitted in a span
type Log struct {
	Timestamp uint64     `json:"timestamp"`
	Fields    []KeyValue `json:"fields"`
}

// KeyValue is a key-value pair with typed value.
type KeyValue struct {
	Key   string    `json:"key"`
	Type  ValueType `json:"type,omitempty"`
	Value any       `json:"value"`
}

// Service is the JSON struct for service:operation documents in ElasticSearch
type Service struct {
	ServiceName   string `json:"serviceName"`
	OperationName string `json:"operationName"`
}
