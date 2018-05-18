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

import (
	"fmt"
)

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

// MaybeAddParentSpanID adds non-zero parentSpanID to refs as a child-of reference.
// We no longer store ParentSpanID in the domain model, but the data in the database
// or other formats might still have these IDs without representing them in the References,
// so this converts parent IDs to canonical reference format.
func MaybeAddParentSpanID(traceID TraceID, parentSpanID SpanID, refs []SpanRef) []SpanRef {
	if parentSpanID == 0 {
		return refs
	}
	for i := range refs {
		r := &refs[i]
		if r.SpanID == parentSpanID && r.TraceID == traceID && r.RefType == ChildOf {
			return refs
		}
	}
	newRef := SpanRef{
		TraceID: traceID,
		SpanID:  parentSpanID,
		RefType: ChildOf,
	}
	if len(refs) == 0 {
		return append(refs, newRef)
	}
	newRefs := make([]SpanRef, len(refs)+1)
	newRefs[0] = newRef
	copy(newRefs[1:], refs)
	return newRefs
}

// NewChildOfRef creates a new child-of span reference.
func NewChildOfRef(traceID TraceID, spanID SpanID) SpanRef {
	return SpanRef{
		RefType: ChildOf,
		TraceID: traceID,
		SpanID:  spanID,
	}
}

// NewFollowsFromRef creates a new follows-from span reference.
func NewFollowsFromRef(traceID TraceID, spanID SpanID) SpanRef {
	return SpanRef{
		RefType: FollowsFrom,
		TraceID: traceID,
		SpanID:  spanID,
	}
}
