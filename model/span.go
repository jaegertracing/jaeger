// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package model

import (
	"encoding/gob"
	"io"
	"unsafe"
)

type SamplerType int

const (
	SamplerTypeUnrecognized SamplerType = iota
	SamplerTypeProbabilistic
	SamplerTypeLowerBound
	SamplerTypeRateLimiting
	SamplerTypeConst
)

var toSamplerType = map[string]SamplerType{
	"unrecognized":  SamplerTypeUnrecognized,
	"probabilistic": SamplerTypeProbabilistic,
	"lowerbound":    SamplerTypeLowerBound,
	"ratelimiting":  SamplerTypeRateLimiting,
	"const":         SamplerTypeConst,
}

func (s SamplerType) String() string {
	switch s {
	case SamplerTypeUnrecognized:
		return "unrecognized"
	case SamplerTypeProbabilistic:
		return "probabilistic"
	case SamplerTypeLowerBound:
		return "lowerbound"
	case SamplerTypeRateLimiting:
		return "ratelimiting"
	case SamplerTypeConst:
		return "const"
	default:
		return ""
	}
}

func SpanKindTag(kind SpanKind) KeyValue {
	return String(SpanKindKey, string(kind))
}

// Hash implements Hash from Hashable.
func (s *Span) Hash(w io.Writer) (err error) {
	// gob is not the most efficient way, but it ensures we don't miss any fields.
	// See BenchmarkSpanHash in span_test.go
	enc := gob.NewEncoder(w)
	return enc.Encode(s)
}

// HasSpanKind returns true if the span has a `span.kind` tag set to `kind`.
func (s *Span) HasSpanKind(kind SpanKind) bool {
	if tag, ok := KeyValues(s.Tags).FindByKey(SpanKindKey); ok {
		return tag.AsString() == string(kind)
	}
	return false
}

// GetSpanKind returns value of `span.kind` tag and whether the tag can be found
func (s *Span) GetSpanKind() (spanKind SpanKind, found bool) {
	if tag, ok := KeyValues(s.Tags).FindByKey(SpanKindKey); ok {
		if kind, err := SpanKindFromString(tag.AsString()); err == nil {
			return kind, true
		}
	}
	return SpanKindUnspecified, false
}

// GetSamplerType returns the sampler type for span
func (s *Span) GetSamplerType() SamplerType {
	// There's no corresponding opentelemetry tag label corresponding to sampler.type
	if tag, ok := KeyValues(s.Tags).FindByKey(SamplerTypeKey); ok {
		if s, ok := toSamplerType[tag.VStr]; ok {
			return s
		}
	}
	return SamplerTypeUnrecognized
}

// IsRPCClient returns true if the span represents a client side of an RPC,
// as indicated by the `span.kind` tag set to `client`.
func (s *Span) IsRPCClient() bool {
	return s.HasSpanKind(SpanKindClient)
}

// IsRPCServer returns true if the span represents a server side of an RPC,
// as indicated by the `span.kind` tag set to `server`.
func (s *Span) IsRPCServer() bool {
	return s.HasSpanKind(SpanKindServer)
}

// GetHashTag returns hash keyValue from Tags and wether the hash was found
func (s *Span) GetHashTag() (hashCode int64, found bool) {
	if tag, ok := KeyValues(s.Tags).FindByKey(SpanHashKey); ok {
		return tag.GetVInt64(), true
	}
	return 0, false
}

func (s *Span) SetHashTag() (hashCode int64, err error) {
	hCode, err := HashCode(s)
	if err != nil {
		return -1, err
	}
	spanHash := uint64ToInt64Bits(hCode)
	s.Tags = append(s.Tags, Int64(SpanHashKey, spanHash))
	return spanHash, err
}

// NormalizeTimestamps changes all timestamps in this span to UTC.
func (s *Span) NormalizeTimestamps() {
	s.StartTime = s.StartTime.UTC()
	for i := range s.Logs {
		s.Logs[i].Timestamp = s.Logs[i].Timestamp.UTC()
	}
}

// ParentSpanID returns ID of a parent span if it exists.
// It searches for the first child-of or follows-from reference pointing to the same trace ID.
func (s *Span) ParentSpanID() SpanID {
	var followsFromRef *SpanRef
	for i := range s.References {
		ref := &s.References[i]
		if ref.TraceID != s.TraceID {
			continue
		}
		if ref.RefType == ChildOf {
			return ref.SpanID
		}
		if followsFromRef == nil && ref.RefType == FollowsFrom {
			followsFromRef = ref
		}
	}
	if followsFromRef != nil {
		return followsFromRef.SpanID
	}
	return SpanID(0)
}

// ReplaceParentID replaces span ID in the parent span reference.
// See also ParentSpanID.
func (s *Span) ReplaceParentID(newParentID SpanID) {
	oldParentID := s.ParentSpanID()
	for i := range s.References {
		if s.References[i].SpanID == oldParentID && s.References[i].TraceID == s.TraceID {
			s.References[i].SpanID = newParentID
			return
		}
	}
	s.References = MaybeAddParentSpanID(s.TraceID, newParentID, s.References)
}

func uint64ToInt64Bits(value uint64) int64 {
	return *(*int64)(unsafe.Pointer(&value))
}
