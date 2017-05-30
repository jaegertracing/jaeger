package dbmodel

import (
	"github.com/uber/jaeger/model"
)


// ReferenceType is the reference type of one span to another
type ReferenceType string

// TraceID is a trace id found in a span
type TraceID string

// SpanID is the id of a span
type SpanID uint64

const (
	// ChildOf means a span is the child of another span
	ChildOf ReferenceType = "CHILD_OF"
	// FollowsFrom means a span follows from another span
	FollowsFrom ReferenceType = "FOLLOWS_FROM"
)

// Span is a span denoting a piece of work in some infrastructure
type Span struct {
	TraceID       TraceID     `json:"traceID"`
	SpanID        SpanID      `json:"spanID"`
	ParentSpanID  SpanID	  `json:"parentSpanID"`
	OperationName string      `json:"operationName"`
	References    []Reference `json:"references"`
	Flags         uint32      `json:"flags"`
	Timestamp     uint64      `json:"timestamp"`
	Duration      uint64      `json:"duration"`
	Tags          []Tag   	  `json:"tags"`
	Logs          []Log       `json:"logs"`
	Process       Process     `json:"process"`
}

// Reference is a reference from one span to another
type Reference struct {
	RefType string 	      `json:"refType"`
	TraceID TraceID       `json:"traceID"`
	SpanID  SpanID        `json:"spanID"`
}

// Process is the process emitting a set of spans
type Process struct {
	ServiceName string `json:"serviceName"`
	Tags        []Tag  `json:"tags"`
}

// Log is a log emitted in a span
type Log struct {
	Timestamp uint64 `json:"timestamp"`
	Tags      []Tag  `json:"tags"`
}

// Tag are arbitrary tags in a span, logs, or a process
type Tag struct {
	Key          string   `json:"key"`
	Value	     string   `json:"value"`
	TagType      string   `json:"tagType"`
}

func TraceIDFromDomain(traceID model.TraceID) TraceID {
	return TraceID(traceID.String())
}

func (dbTraceID TraceID) TraceIDToDomain() (model.TraceID, error) {
	traceIDstr, err := model.TraceIDFromString(string(dbTraceID))
	return traceIDstr, err
}