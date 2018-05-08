package protomodel

import (
	"fmt"
)

// MarshalJSON renders trace id as a single hex string.
func (t TraceID) MarshalJSON() ([]byte, error) {
	if t.High == 0 {
		return []byte(fmt.Sprintf(`"%016x"`, t.Low)), nil
	}
	return []byte(fmt.Sprintf(`"%016x%016x"`, t.High, t.Low)), nil
}

// func (t *TraceID) UnmarshalJSON(data []byte) error {
// // TODO
// }

// MarshalJSON renders span id as a single hex string.
func (t SpanID) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf(`"%016x"`, t)), nil
}

type SpanID uint64

// NewSpanID creates new span ID from uint64.
func NewSpanID(id uint64) SpanID {
	return SpanID(id)
}
