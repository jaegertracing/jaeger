// Copyright (c) 2022 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package sanitizer

import (
	"unsafe"

	"github.com/jaegertracing/jaeger/model"
)

// NewHashingSanitizer creates a sanitizer to add hash field to spans
func NewHashingSanitizer() SanitizeSpan {
	return hashingSanitizer
}

func hashingSanitizer(span *model.Span) *model.Span {
	// Check if hash already exists
	if found := span.HashHashTag(); found {
		return span
	}

	spanHash, err := model.HashCode(span)
	if err != nil {
		return span
	}

	span.Tags = append(span.Tags, model.KeyValue{
		Key:    model.SpanHashKey,
		VType:  model.ValueType_INT64,
		VInt64: uint64ToInt64Bits(spanHash),
	})

	return span
}

func uint64ToInt64Bits(value uint64) int64 {
	return *(*int64)(unsafe.Pointer(&value))
}
