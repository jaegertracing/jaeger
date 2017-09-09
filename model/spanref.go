// Copyright (c) 2017 Uber Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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
