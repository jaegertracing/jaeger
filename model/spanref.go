// Copyright (c) 2016 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package model

import "fmt"

// SpanRefType describes the type of a span reference
type SpanRefType int

const (
	// ChildOf span reference type describes a reference to a parent span
	// that depends on the response from the current (child) span
	ChildOf SpanRefType = iota

	// FollowsFrom span reference type describes a reference to a "parent" span
	// that does not depend on the response from the current (child) span
	FollowsFrom

	childOfStr     = "child-of"
	followsFromStr = "follows-from"
)

// SpanRef describes a reference from one span to another
type SpanRef struct {
	RefType SpanRefType `json:"refType"`
	TraceID TraceID     `json:"traceID"`
	SpanID  SpanID      `json:"spanID"`
}

func (p SpanRefType) String() string {
	switch p {
	case ChildOf:
		return childOfStr
	case FollowsFrom:
		return followsFromStr
	}
	return "<invalid>"
}

// SpanRefTypeFromString converts a string into SpanRefType enum.
func SpanRefTypeFromString(s string) (SpanRefType, error) {
	switch s {
	case childOfStr:
		return ChildOf, nil
	case followsFromStr:
		return FollowsFrom, nil
	}
	return SpanRefType(0), fmt.Errorf("not a valid SpanRefType string %s", s)
}

// MarshalText allows SpanRefType to serialize itself in JSON as a string.
func (p SpanRefType) MarshalText() ([]byte, error) {
	return []byte(p.String()), nil
}

// UnmarshalText allows SpanRefType to deserialize itself from a JSON string.
func (p *SpanRefType) UnmarshalText(text []byte) error {
	q, err := SpanRefTypeFromString(string(text))
	if err != nil {
		return err
	}
	*p = q
	return nil
}
