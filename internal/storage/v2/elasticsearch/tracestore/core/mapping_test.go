// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	builder "github.com/jaegertracing/jaeger/internal/expression"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/esclient"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/elasticsearch/tracestore/core/dbmodel"
)

func spanMapping(withNumber bool) esclient.Mapping {
	value := esclient.Mapping{Type: "keyword"}
	if withNumber {
		value.Fields = map[string]esclient.Mapping{"number": {Type: "double"}}
	}
	return esclient.Mapping{Properties: map[string]esclient.Mapping{
		"tags": {Properties: map[string]esclient.Mapping{"value": value}},
	}}
}

func orderedAttributeQuery(filter *builder.Predicate) dbmodel.TraceQueryParameters {
	return dbmodel.TraceQueryParameters{
		Filter:       filter.Span().Attr("retry.count").Gt("10"),
		StartTimeMin: time.Date(2026, time.September, 29, 10, 0, 0, 0, time.UTC),
		StartTimeMax: time.Date(2026, time.September, 29, 11, 0, 0, 0, time.UTC),
	}
}

func TestSpanReaderFindTraceIDsRefusesOrderedAttributeWithoutNumericMapping(t *testing.T) {
	setTypedAttributeIndexing(t, true)
	var p builder.Predicate

	withSpanReader(t, func(r *spanReaderTest) {
		mappingCalls := 0
		r.reader.getMappings = func(_ context.Context, indices []string) (map[string]esclient.IndexMapping, error) {
			mappingCalls++
			require.Equal(t, []string{"jaeger-span-2026-09-29"}, indices)
			return map[string]esclient.IndexMapping{
				"jaeger-span-old": {Mappings: spanMapping(false)},
				"jaeger-span-new": {Mappings: spanMapping(true)},
			}, nil
		}

		ids, err := r.reader.FindTraceIDs(context.Background(), orderedAttributeQuery(&p))
		require.ErrorIs(t, err, tracestore.ErrFilterUnsupported)
		require.ErrorContains(t, err, `indexes "retry.count" as a keyword rather than a number`)
		assert.Nil(t, ids)
		assert.Equal(t, 1, mappingCalls)
		r.searcher.AssertNotCalled(t, "Search")
	})
}

func TestSpanReaderFindTraceIDsSearchesWhenOrderedAttributeHasNumericMapping(t *testing.T) {
	setTypedAttributeIndexing(t, true)
	var p builder.Predicate

	withSpanReader(t, func(r *spanReaderTest) {
		r.reader.getMappings = func(_ context.Context, _ []string) (map[string]esclient.IndexMapping, error) {
			return map[string]esclient.IndexMapping{
				"jaeger-span-new": {Mappings: spanMapping(true)},
			}, nil
		}
		mockSearchService(r).Return(&esclient.SearchResponse{}, nil)

		ids, err := r.reader.FindTraceIDs(context.Background(), orderedAttributeQuery(&p))
		require.NoError(t, err)
		assert.Empty(t, ids)
	})
}

func TestSpanReaderFindTraceIDsReturnsMappingLookupError(t *testing.T) {
	setTypedAttributeIndexing(t, true)
	var p builder.Predicate

	withSpanReader(t, func(r *spanReaderTest) {
		r.reader.getMappings = func(_ context.Context, _ []string) (map[string]esclient.IndexMapping, error) {
			return nil, errors.New("mapping unavailable")
		}

		ids, err := r.reader.FindTraceIDs(context.Background(), orderedAttributeQuery(&p))
		require.ErrorContains(t, err, "get span index mappings: mapping unavailable")
		assert.Nil(t, ids)
		r.searcher.AssertNotCalled(t, "Search")
	})
}

func TestSpanReaderFindTraceIDsRequiresMappingReaderForOrderedAttribute(t *testing.T) {
	setTypedAttributeIndexing(t, true)
	var p builder.Predicate

	withSpanReader(t, func(r *spanReaderTest) {
		ids, err := r.reader.FindTraceIDs(context.Background(), orderedAttributeQuery(&p))
		require.ErrorContains(t, err, "ordered attribute mapping reader is not configured")
		assert.Nil(t, ids)
		r.searcher.AssertNotCalled(t, "Search")
	})
}

func TestMappingSupportsOrderedAttribute(t *testing.T) {
	withSpanReader(t, func(r *spanReaderTest) {
		ref := reference{name: "retry.count", level: "span", attribute: true}
		assert.False(t, r.reader.mappingSupportsOrderedAttribute(spanMapping(false), ref))
		assert.True(t, r.reader.mappingSupportsOrderedAttribute(spanMapping(true), ref))

		mapping := spanMapping(true)
		mapping.Properties["tag"] = esclient.Mapping{Properties: map[string]esclient.Mapping{
			"retry@count": {Type: "keyword"},
		}}
		assert.False(t, r.reader.mappingSupportsOrderedAttribute(mapping, ref))
		assert.False(t, r.reader.mappingSupportsOrderedAttribute(esclient.Mapping{}, reference{level: "scope", attribute: true}))
	})
}
