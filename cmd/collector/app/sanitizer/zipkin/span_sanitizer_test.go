// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package zipkin

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaegertracing/jaeger-idl/thrift-gen/zipkincore"
	"github.com/jaegertracing/jaeger/pkg/testutils"
)

var (
	negativeDuration = int64(-1)
	positiveDuration = int64(1)
)

func TestNewStandardSanitizers(*testing.T) {
	NewStandardSanitizers()
}

func TestChainedSanitizer(t *testing.T) {
	sanitizer := NewChainedSanitizer(NewSpanDurationSanitizer())

	span := &zipkincore.Span{Duration: &negativeDuration}
	actual := sanitizer.Sanitize(span)
	assert.Equal(t, positiveDuration, *actual.Duration)
}

func TestSpanDurationSanitizer(t *testing.T) {
	sanitizer := NewSpanDurationSanitizer()

	span := &zipkincore.Span{Duration: &negativeDuration}
	actual := sanitizer.Sanitize(span)
	assert.Equal(t, positiveDuration, *actual.Duration)
	assert.Len(t, actual.BinaryAnnotations, 1)
	assert.Equal(t, "-1", string(actual.BinaryAnnotations[0].Value))

	sanitizer = NewSpanDurationSanitizer()
	span = &zipkincore.Span{Duration: &positiveDuration}
	actual = sanitizer.Sanitize(span)
	assert.Equal(t, positiveDuration, *actual.Duration)
	assert.Empty(t, actual.BinaryAnnotations)

	sanitizer = NewSpanDurationSanitizer()
	nilDurationSpan := &zipkincore.Span{}
	actual = sanitizer.Sanitize(nilDurationSpan)
	assert.Equal(t, int64(1), *actual.Duration)

	span = &zipkincore.Span{
		Annotations: []*zipkincore.Annotation{
			{Value: zipkincore.CLIENT_SEND, Timestamp: 10},
			{Value: zipkincore.CLIENT_RECV, Timestamp: 30},
		},
	}
	actual = sanitizer.Sanitize(span)
	assert.Equal(t, int64(20), *actual.Duration)

	span = &zipkincore.Span{
		Annotations: []*zipkincore.Annotation{
			{Value: "first", Timestamp: 100},
			{Value: zipkincore.CLIENT_SEND, Timestamp: 10},
			{Value: zipkincore.CLIENT_RECV, Timestamp: 30},
			{Value: "last", Timestamp: 300},
		},
	}
	actual = sanitizer.Sanitize(span)
	assert.Equal(t, int64(20), *actual.Duration)
}

func TestSpanParentIDSanitizer(t *testing.T) {
	var (
		zero = int64(0)
		four = int64(4)
	)
	tests := []struct {
		parentID *int64
		expected *int64
		tag      bool
		descr    string
	}{
		{&zero, nil, true, "zero"},
		{&four, &four, false, "four"},
		{nil, nil, false, "nil"},
	}
	for _, test := range tests {
		span := &zipkincore.Span{
			ParentID: test.parentID,
		}
		sanitizer := NewParentIDSanitizer()
		actual := sanitizer.Sanitize(span)
		assert.Equal(t, test.expected, actual.ParentID)
		if test.tag {
			if assert.Len(t, actual.BinaryAnnotations, 1) {
				assert.Equal(t, "0", string(actual.BinaryAnnotations[0].Value))
				assert.Equal(t, zeroParentIDTag, string(actual.BinaryAnnotations[0].Key))
			}
		} else {
			assert.Empty(t, actual.BinaryAnnotations)
		}
	}
}

func TestSpanErrorSanitizer(t *testing.T) {
	sanitizer := NewErrorTagSanitizer()

	tests := []struct {
		binAnn        *zipkincore.BinaryAnnotation
		isErrorTag    bool
		isError       bool
		addErrMsgAnno bool
	}{
		// value is string
		{
			&zipkincore.BinaryAnnotation{Key: "error", AnnotationType: zipkincore.AnnotationType_STRING},
			true, true, false,
		},
		{
			&zipkincore.BinaryAnnotation{Key: "error", Value: []byte("true"), AnnotationType: zipkincore.AnnotationType_STRING},
			true, true, false,
		},
		{
			&zipkincore.BinaryAnnotation{Key: "error", Value: []byte("message"), AnnotationType: zipkincore.AnnotationType_STRING},
			true, true, true,
		},
		{
			&zipkincore.BinaryAnnotation{Key: "error", Value: []byte("false"), AnnotationType: zipkincore.AnnotationType_STRING},
			true, false, false,
		},
	}

	for _, test := range tests {
		span := &zipkincore.Span{
			BinaryAnnotations: []*zipkincore.BinaryAnnotation{test.binAnn},
		}

		sanitized := sanitizer.Sanitize(span)
		if test.isErrorTag {
			expectedVal := []byte{0}
			if test.isError {
				expectedVal = []byte{1}
			}

			assert.Equal(t, expectedVal, sanitized.BinaryAnnotations[0].Value, test.binAnn.Key)
			assert.Equal(t, zipkincore.AnnotationType_BOOL, sanitized.BinaryAnnotations[0].AnnotationType)

			if test.addErrMsgAnno {
				assert.Len(t, sanitized.BinaryAnnotations, 2)
				assert.Equal(t, "error.message", sanitized.BinaryAnnotations[1].Key)
				assert.Equal(t, "message", string(sanitized.BinaryAnnotations[1].Value))
				assert.Equal(t, zipkincore.AnnotationType_STRING, sanitized.BinaryAnnotations[1].AnnotationType)
			} else {
				assert.Len(t, sanitized.BinaryAnnotations, 1)
			}
		}
	}
}

func TestSpanStartTimeSanitizer(t *testing.T) {
	sanitizer := NewSpanStartTimeSanitizer()

	var helper int64 = 30
	span := &zipkincore.Span{
		Timestamp: &helper,
		Annotations: []*zipkincore.Annotation{
			{Value: zipkincore.CLIENT_SEND, Timestamp: 10},
			{Value: zipkincore.SERVER_RECV, Timestamp: 20},
		},
	}
	sanitized := sanitizer.Sanitize(span)
	assert.Equal(t, int64(30), *sanitized.Timestamp)

	span = &zipkincore.Span{
		Annotations: []*zipkincore.Annotation{
			{Value: zipkincore.CLIENT_SEND, Timestamp: 10},
			{Value: zipkincore.SERVER_RECV, Timestamp: 20},
		},
	}
	sanitized = sanitizer.Sanitize(span)
	assert.Equal(t, int64(10), *sanitized.Timestamp)
	span = &zipkincore.Span{
		Annotations: []*zipkincore.Annotation{
			{Value: zipkincore.SERVER_SEND, Timestamp: 10},
			{Value: zipkincore.SERVER_RECV, Timestamp: 20},
		},
	}
	sanitized = sanitizer.Sanitize(span)
	assert.Equal(t, int64(20), *sanitized.Timestamp)
}

func TestMain(m *testing.M) {
	testutils.VerifyGoLeaks(m)
}
