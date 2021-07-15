// Copyright (c) 2019 The Jaeger Authors.
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
	"strconv"

	"github.com/opentracing/opentracing-go/ext"
	"github.com/uber/jaeger-client-go"
	"go.uber.org/zap"
)

const (
	// SampledFlag is the bit set in Flags in order to define a span as a sampled span
	SampledFlag = Flags(1)
	// DebugFlag is the bit set in Flags in order to define a span as a debug span
	DebugFlag = Flags(2)
	// FirehoseFlag is the bit in Flags in order to define a span as a firehose span
	FirehoseFlag = Flags(8)

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

// GetSpanKind returns value of `span.kind` tag and whether the tag can be found
func (s *Span) GetSpanKind() (spanKind string, found bool) {
	if tag, ok := KeyValues(s.Tags).FindByKey(string(ext.SpanKind)); ok {
		return tag.AsString(), true
	}
	return "", false
}

// GetSamplerType returns the sampler type for span
func (s *Span) GetSamplerType() string {
	// There's no corresponding opentracing-go tag label corresponding to sampler.type
	if tag, ok := KeyValues(s.Tags).FindByKey(samplerType); ok {
		if tag.VStr == "" {
			return samplerTypeUnknown
		}
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

// GetSamplerParams returns the sampler.type and sampler.param value if they are valid.
func (s *Span) GetSamplerParams(logger *zap.Logger) (string, float64) {
	tag, ok := KeyValues(s.Tags).FindByKey(jaeger.SamplerTypeTagKey)
	if !ok {
		return "", 0
	}
	if tag.VType != StringType {
		logger.
			With(zap.String("traceID", s.TraceID.String())).
			With(zap.String("spanID", s.SpanID.String())).
			Warn("sampler.type tag is not a string", zap.Any("tag", tag))
		return "", 0
	}
	samplerType := tag.AsString()
	if samplerType != jaeger.SamplerTypeProbabilistic && samplerType != jaeger.SamplerTypeLowerBound &&
		samplerType != jaeger.SamplerTypeRateLimiting {
		return "", 0
	}
	tag, ok = KeyValues(s.Tags).FindByKey(jaeger.SamplerParamTagKey)
	if !ok {
		return "", 0
	}
	samplerParam, err := samplerParamToFloat(tag)
	if err != nil {
		logger.
			With(zap.String("traceID", s.TraceID.String())).
			With(zap.String("spanID", s.SpanID.String())).
			Warn("sampler.param tag is not a number", zap.Any("tag", tag))
		return "", 0
	}
	return samplerType, samplerParam
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

// SetFirehose set the Flags as firehose enabled
func (f *Flags) SetFirehose() {
	f.setFlags(FirehoseFlag)
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

// IsFirehoseEnabled returns true if firehose is enabled
// Firehose is used to decide whether to index a span or not
func (f Flags) IsFirehoseEnabled() bool {
	return f.checkFlags(FirehoseFlag)
}

func (f Flags) checkFlags(bit Flags) bool {
	return f&bit == bit
}

func samplerParamToFloat(samplerParamTag KeyValue) (float64, error) {
	// The param could be represented as a string, an int, or a float
	switch samplerParamTag.VType {
	case Float64Type:
		return samplerParamTag.Float64(), nil
	case Int64Type:
		return float64(samplerParamTag.Int64()), nil
	default:
		return strconv.ParseFloat(samplerParamTag.AsString(), 64)
	}
}
