package model

// TraceID is a random 128bit identifier for a trace
type TraceID struct {
	Low  uint64 `json:"lo"`
	High uint64 `json:"hi"`
}

// SpanID is a random 64bit identifier for a span
type SpanID uint64

// Span represents a unit of work in an application, such as an RPC, a database call, etc.
type Span struct {
	TraceID       TraceID   `json:"traceId"`
	SpanID        SpanID    `json:"spanId"`
	ParentSpanID  SpanID    `json:"parentSpanId"`
	OperationName string    `json:"operationName"`
	References    []SpanRef `json:"references,omitempty"`
	Flags         uint32    `json:"flags"`
	StartTime     uint64    `json:"startTime"`
	Duration      uint64    `json:"duration"`
	Tags          []Tag     `json:"tags,omitempty"`
	Logs          []Log     `json:"logs,omitempty"`
	Process       *Process  `json:"process"`
}
