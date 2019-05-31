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
	"encoding/gob"
	"io"

	"github.com/opentracing/opentracing-go/ext"
)

const (
	// SampledFlag is the bit set in Flags in order to define a span as a sampled span
	SampledFlag = Flags(1)
	// DebugFlag is the bit set in Flags in order to define a span as a debug span
	DebugFlag = Flags(2)

	samplerType        = "sampler.type"
	samplerTypeUnknown = "unknown"
)

// Flags is a bit map of flags for a span
type Flags uint32

// Hash implements Hash from Hashable.
func (s *Span) Hash(w io.Writer) (err error) {
	// gob is not the most efficient way, but it ensures we don't miss any fields.
	// See BenchmarkSpanHash in span_test.go
	enc := gob.NewEncoder(w)
	return enc.Encode(s)
}

// HasSpanKind returns true if the span has a `span.kind` tag set to `kind`.
func (s *Span) HasSpanKind(kind ext.SpanKindEnum) bool {
	if tag, ok := KeyValues(s.Tags).FindByKey(string(ext.SpanKind)); ok {
		return tag.AsString() == string(kind)
	}
	return false
}

// GetSamplerType returns the sampler type for span
func (s *Span) GetSamplerType() string {
	// There's no corresponding opentracing-go tag label corresponding to sampler.type
	if tag, ok := KeyValues(s.Tags).FindByKey(samplerType); ok {
		return tag.VStr
	}
	return samplerTypeUnknown
}

// IsRPCClient returns true if the span represents a client side of an RPC,
// as indicated by the `span.kind` tag set to `client`.
func (s *Span) IsRPCClient() bool {
	return s.HasSpanKind(ext.SpanKindRPCClientEnum)
}

// IsRPCServer returns true if the span represents a server side of an RPC,
// as indicated by the `span.kind` tag set to `server`.
func (s *Span) IsRPCServer() bool {
	return s.HasSpanKind(ext.SpanKindRPCServerEnum)
}

// NormalizeTimestamps changes all timestamps in this span to UTC.
func (s *Span) NormalizeTimestamps() {
	s.StartTime = s.StartTime.UTC()
	for i := range s.Logs {
		s.Logs[i].Timestamp = s.Logs[i].Timestamp.UTC()
	}
}

// ParentSpanID returns ID of a parent span if it exists.
// It searches for the first child-of reference pointing to the same trace ID.
func (s *Span) ParentSpanID() SpanID {
	for i := range s.References {
		ref := &s.References[i]
		if ref.TraceID == s.TraceID && ref.RefType == ChildOf {
			return ref.SpanID
		}
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

// ------- Flags -------

// SetSampled sets the Flags as sampled
func (f *Flags) SetSampled() {
	f.setFlags(SampledFlag)
}

// SetDebug set the Flags as sampled
func (f *Flags) SetDebug() {
	f.setFlags(DebugFlag)
}

func (f *Flags) setFlags(bit Flags) {
	*f = *f | bit
}

// IsSampled returns true if the Flags denote sampling
func (f Flags) IsSampled() bool {
	return f.checkFlags(SampledFlag)
}

// IsDebug returns true if the Flags denote debugging
// Debugging can be useful in testing tracing availability or correctness
func (f Flags) IsDebug() bool {
	return f.checkFlags(DebugFlag)
}

func (f Flags) checkFlags(bit Flags) bool {
	return f&bit == bit
}
