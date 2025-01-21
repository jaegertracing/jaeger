// Copyright (c) 2024 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package jptrace

import (
	"errors"
	"fmt"
	"hash/fnv"
	"unsafe"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

type SpanHasher struct {
	marshaler ptrace.Marshaler
}

func NewSpanHasher() *SpanHasher {
	return &SpanHasher{
		marshaler: &ptrace.ProtoMarshaler{},
	}
}

func (s *SpanHasher) SpanHash(trace ptrace.Traces) (int64, error) {
	var spanHash int64
	span := trace.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0)
	if hashAttr, ok := span.Attributes().Get(HashAttribute); ok {
		if hashAttr.Type() != pcommon.ValueTypeInt {
			return 0, errors.New("span hash attribute is not an integer type")
		}
		spanHash = hashAttr.Int()
	} else {
		h, err := s.computeHashCode(trace)
		if err != nil {
			return 0, fmt.Errorf("failed to compute hash code: %w", err)
		}
		spanHash = uint64ToInt64Bits(h)
	}

	return spanHash, nil
}

func (s *SpanHasher) computeHashCode(
	hashTrace ptrace.Traces,
) (uint64, error) {
	b, err := s.marshaler.MarshalTraces(hashTrace)
	if err != nil {
		return 0, err
	}
	hasher := fnv.New64a()
	hasher.Write(b) // the writer in the Hash interface never returns an error
	return hasher.Sum64(), nil
}

func uint64ToInt64Bits(value uint64) int64 {
	return *(*int64)(unsafe.Pointer(&value))
}
