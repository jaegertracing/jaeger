// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package zipkin

import (
	"strconv"
	"strings"

	zc "github.com/jaegertracing/jaeger-idl/thrift-gen/zipkincore"
)

const (
	negativeDurationTag = "errNegativeDuration"
	zeroParentIDTag     = "errZeroParentID"
)

var defaultDuration = int64(1) // not a const because we take its address

// NewStandardSanitizers is a list of standard zipkin sanitizers.
func NewStandardSanitizers() []Sanitizer {
	return []Sanitizer{
		NewSpanStartTimeSanitizer(),
		NewSpanDurationSanitizer(),
		NewParentIDSanitizer(),
		NewErrorTagSanitizer(),
	}
}

// Sanitizer interface for sanitizing spans. Any business logic that needs to be applied to normalize the contents of a
// span should implement this interface.
// TODO - just make this a function
type Sanitizer interface {
	Sanitize(span *zc.Span) *zc.Span
}

// ChainedSanitizer applies multiple sanitizers in serial fashion
type ChainedSanitizer []Sanitizer

// NewChainedSanitizer creates a Sanitizer from the variadic list of passed Sanitizers
func NewChainedSanitizer(sanitizers ...Sanitizer) ChainedSanitizer {
	return sanitizers
}

// Sanitize calls each Sanitize, returning the first error
func (cs ChainedSanitizer) Sanitize(span *zc.Span) *zc.Span {
	for _, s := range cs {
		span = s.Sanitize(span)
	}
	return span
}

// NewSpanDurationSanitizer returns a sanitizer that deals with nil or 0 span duration.
func NewSpanDurationSanitizer() Sanitizer {
	return &spanDurationSanitizer{}
}

type spanDurationSanitizer struct{}

func (*spanDurationSanitizer) Sanitize(span *zc.Span) *zc.Span {
	if span.Duration == nil {
		duration := defaultDuration
		if len(span.Annotations) >= 2 {
			// Prefer RPC one-way (cs -> sr) vs arbitrary annotations.
			first := span.Annotations[0].Timestamp
			last := span.Annotations[len(span.Annotations)-1].Timestamp
			for _, anno := range span.Annotations {
				if anno.Value == zc.CLIENT_SEND {
					first = anno.Timestamp
				} else if anno.Value == zc.CLIENT_RECV {
					last = anno.Timestamp
				}
			}
			if first < last {
				duration = last - first
				if span.Timestamp == nil {
					span.Timestamp = &first
				}
			}
		}
		span.Duration = &duration
		return span
	}

	duration := *span.Duration
	if duration >= 0 {
		return span
	}
	span.Duration = &defaultDuration
	annotation := zc.BinaryAnnotation{
		Key:            negativeDurationTag,
		Value:          []byte(strconv.FormatInt(duration, 10)),
		AnnotationType: zc.AnnotationType_STRING,
	}
	span.BinaryAnnotations = append(span.BinaryAnnotations, &annotation)
	return span
}

// NewSpanStartTimeSanitizer returns a Sanitizer that changes span start time if is nil
// If there is zipkincore.CLIENT_SEND use that, if no fall back on zipkincore.SERVER_RECV
func NewSpanStartTimeSanitizer() Sanitizer {
	return &spanStartTimeSanitizer{}
}

type spanStartTimeSanitizer struct{}

func (*spanStartTimeSanitizer) Sanitize(span *zc.Span) *zc.Span {
	if span.Timestamp != nil || len(span.Annotations) == 0 {
		return span
	}

	for _, anno := range span.Annotations {
		if anno.Value == zc.CLIENT_SEND {
			span.Timestamp = &anno.Timestamp
			return span
		}
		if anno.Value == zc.SERVER_RECV && span.ParentID == nil {
			// continue, cs has higher precedence and might be after
			span.Timestamp = &anno.Timestamp
		}
	}

	return span
}

// NewParentIDSanitizer returns a sanitizer that deals parentID == 0
// by replacing with nil, per Zipkin convention.
func NewParentIDSanitizer() Sanitizer {
	return &parentIDSanitizer{}
}

type parentIDSanitizer struct{}

func (*parentIDSanitizer) Sanitize(span *zc.Span) *zc.Span {
	if span.ParentID == nil || *span.ParentID != 0 {
		return span
	}
	annotation := zc.BinaryAnnotation{
		Key:            zeroParentIDTag,
		Value:          []byte("0"),
		AnnotationType: zc.AnnotationType_STRING,
	}
	span.BinaryAnnotations = append(span.BinaryAnnotations, &annotation)
	span.ParentID = nil
	return span
}

// NewErrorTagSanitizer returns a sanitizer that changes error binary annotations to boolean type
// and sets appropriate value, in case value was a string message it adds a 'error.message' binary annotation with
// this message.
func NewErrorTagSanitizer() Sanitizer {
	return &errorTagSanitizer{}
}

type errorTagSanitizer struct{}

func (*errorTagSanitizer) Sanitize(span *zc.Span) *zc.Span {
	for _, binAnno := range span.BinaryAnnotations {
		if binAnno.AnnotationType != zc.AnnotationType_BOOL && strings.EqualFold("error", binAnno.Key) {
			binAnno.AnnotationType = zc.AnnotationType_BOOL

			switch {
			case len(binAnno.Value) == 0 || strings.EqualFold("true", string(binAnno.Value)):
				binAnno.Value = []byte{1}
			case strings.EqualFold("false", string(binAnno.Value)):
				binAnno.Value = []byte{0}
			default:
				// value is different to true/false, create another bin annotation with error message
				annoErrorMsg := &zc.BinaryAnnotation{
					Key:            "error.message",
					Value:          binAnno.Value,
					AnnotationType: zc.AnnotationType_STRING,
				}
				span.BinaryAnnotations = append(span.BinaryAnnotations, annoErrorMsg)
				binAnno.Value = []byte{1}
			}
		}
	}

	return span
}
