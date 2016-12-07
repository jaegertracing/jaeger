package model

// SpanRefType describes the type of a span reference
type SpanRefType int

const (
	// ChildOf span reference type describes a reference to a parent span
	// that depends on the response from the current (child) span
	ChildOf SpanRefType = iota

	// FollowsFrom span reference type describes a reference to a "parent" span
	// that does not depend on the response from the current (child) span
	FollowsFrom
)

// SpanRef describes a reference from one span to another
type SpanRef struct {
	RefType SpanRefType `json:"refType"`
	TraceID TraceID     `json:"traceId"`
	SpanID  SpanID      `json:"spanId"`
}
