package model

// Trace is a directed acyclic graph of Spans
type Trace struct {
	Spans []*Span
}
