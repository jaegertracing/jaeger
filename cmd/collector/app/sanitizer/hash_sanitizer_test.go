// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package sanitizer

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/model"
)

func TestHashingSanitizer(t *testing.T) {
	sanitizer := NewHashingSanitizer()

	t.Run("add hash to span without hash tag", func(t *testing.T) {
		span := &model.Span{
			TraceID: model.TraceID{Low: 1},
			SpanID:  model.SpanID(2),
		}
		sanitizedSpan := sanitizer(span)
		_, exists := sanitizedSpan.GetHashTag()
		assert.True(t, exists, "hash tag should be added")
	})

	t.Run("preserve existing hash tag", func(t *testing.T) {
		span := &model.Span{
			TraceID: model.TraceID{Low: 1},
			SpanID:  model.SpanID(2),
			Tags: []model.KeyValue{
				{
					Key:    "span.hash",
					VType:  model.ValueType_INT64,
					VInt64: 12345,
				},
			},
		}
		sanitizedSpan := sanitizer(span)
		hash, exists := sanitizedSpan.GetHashTag()
		assert.True(t, exists, "hash tag should exist")
		assert.Equal(t, int64(12345), hash, "hash value should remain unchanged")
	})

	t.Run("identical spans get same hash", func(t *testing.T) {
		span1 := &model.Span{
			TraceID: model.TraceID{Low: 1},
			SpanID:  model.SpanID(2),
		}
		span2 := &model.Span{
			TraceID: model.TraceID{Low: 1},
			SpanID:  model.SpanID(2),
		}

		sanitizedSpan1 := sanitizer(span1)
		sanitizedSpan2 := sanitizer(span2)

		hash1, exists1 := sanitizedSpan1.GetHashTag()
		hash2, exists2 := sanitizedSpan2.GetHashTag()

		require.True(t, exists1, "hash tag should be added to span1")
		require.True(t, exists2, "hash tag should be added to span2")
		assert.Equal(t, hash1, hash2, "identical spans should have same hash")
	})
}
