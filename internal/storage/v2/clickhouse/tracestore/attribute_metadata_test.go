// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package tracestore

import (
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"

	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/clickhousetest"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/sql"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/tracestore/dbmodel"
)

func TestGetAttributeMetadata_ErrorCases(t *testing.T) {
	attrs := pcommon.NewMap()
	attrs.PutStr("http.method", "GET")

	tests := []struct {
		name        string
		driver      *clickhousetest.Driver
		expectedErr string
	}{
		{
			name: "QueryError",
			driver: &clickhousetest.Driver{
				T: t,
				QueryResponses: map[string]*clickhousetest.QueryResponse{
					sql.SelectAttributeMetadata: {
						Rows: nil,
						Err:  assert.AnError,
					},
				},
			},
			expectedErr: "failed to query attribute metadata",
		},
		{
			name: "ScanStructError",
			driver: &clickhousetest.Driver{
				T: t,
				QueryResponses: map[string]*clickhousetest.QueryResponse{
					sql.SelectAttributeMetadata: {
						Rows: &clickhousetest.Rows[dbmodel.AttributeMetadata]{
							Data: []dbmodel.AttributeMetadata{{
								AttributeKey: "http.method",
								Type:         "str",
								Level:        "span",
							}},
							ScanErr: assert.AnError,
						},
						Err: nil,
					},
				},
			},
			expectedErr: "failed to scan row",
		},
		{
			name: "RowsIterationError",
			driver: &clickhousetest.Driver{
				T: t,
				QueryResponses: map[string]*clickhousetest.QueryResponse{
					sql.SelectAttributeMetadata: {
						Rows: &clickhousetest.Rows[dbmodel.AttributeMetadata]{
							Data: []dbmodel.AttributeMetadata{{
								AttributeKey: "http.method",
								Type:         "str",
								Level:        "span",
							}},
							ScanFn: func(dest any, src dbmodel.AttributeMetadata) error {
								ptr, ok := dest.(*dbmodel.AttributeMetadata)
								if !ok {
									return assert.AnError
								}
								*ptr = src
								return nil
							},
							RowsErr: assert.AnError,
						},
						Err: nil,
					},
				},
			},
			expectedErr: "error iterating attribute metadata rows",
		},
		{
			name: "UnknownLevelError",
			driver: &clickhousetest.Driver{
				T: t,
				QueryResponses: map[string]*clickhousetest.QueryResponse{
					sql.SelectAttributeMetadata: {
						Rows: &clickhousetest.Rows[dbmodel.AttributeMetadata]{
							Data: []dbmodel.AttributeMetadata{{
								AttributeKey: "http.method",
								Type:         "str",
								Level:        "unknown",
							}},
							ScanFn: func(dest any, src dbmodel.AttributeMetadata) error {
								ptr, ok := dest.(*dbmodel.AttributeMetadata)
								if !ok {
									return assert.AnError
								}
								*ptr = src
								return nil
							},
						},
						Err: nil,
					},
				},
			},
			expectedErr: "unknown attribute level",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := NewReader(tt.driver, ReaderConfig{AttributeMetadataCacheMaxSize: 1000})
			_, err := reader.getAttributeMetadata(t.Context(), attrs)
			require.Error(t, err)
			assert.ErrorContains(t, err, tt.expectedErr)
		})
	}
}

func TestGetAttributeMetadata_NoStringAttributes(t *testing.T) {
	attrs := pcommon.NewMap()
	attrs.PutBool("some.bool", true)
	attrs.PutInt("some.int", 42)
	attrs.PutDouble("some.double", 3.14)

	driver := &clickhousetest.Driver{
		T: t,
	}

	reader := NewReader(driver, ReaderConfig{AttributeMetadataCacheMaxSize: 1000})
	metadata, err := reader.getAttributeMetadata(t.Context(), attrs)
	require.NoError(t, err)
	assert.Empty(t, metadata)
	assert.Empty(t, driver.RecordedQueries)
}

func makeTestDriverWithMetadata(t *testing.T) *clickhousetest.Driver {
	return &clickhousetest.Driver{
		T: t,
		QueryResponses: map[string]*clickhousetest.QueryResponse{
			sql.SelectAttributeMetadata: {
				Rows: &clickhousetest.Rows[dbmodel.AttributeMetadata]{
					Data: []dbmodel.AttributeMetadata{
						{AttributeKey: "http.method", Type: "str", Level: "span"},
					},
					ScanFn: func(dest any, src dbmodel.AttributeMetadata) error {
						ptr, ok := dest.(*dbmodel.AttributeMetadata)
						if !ok {
							return assert.AnError
						}
						*ptr = src
						return nil
					},
				},
			},
		},
	}
}

func TestGetAttributeMetadata_CacheMiss(t *testing.T) {
	d := makeTestDriverWithMetadata(t)
	reader := NewReader(d, ReaderConfig{
		AttributeMetadataCacheTTL:     time.Minute,
		AttributeMetadataCacheMaxSize: 1000,
	})

	attrs := pcommon.NewMap()
	attrs.PutStr("http.method", "GET")

	// Key is not in cache, should query ClickHouse
	metadata, err := reader.getAttributeMetadata(t.Context(), attrs)
	require.NoError(t, err)
	assert.Len(t, metadata, 1)
	assert.Len(t, d.RecordedQueries, 1, "expected query to ClickHouse on cache miss")
	assert.Equal(t, 1, reader.attrMetaCache.Size(), "result should be cached after miss")
}

func TestGetAttributeMetadata_CacheHit(t *testing.T) {
	d := makeTestDriverWithMetadata(t)
	reader := NewReader(d, ReaderConfig{
		AttributeMetadataCacheTTL:     time.Minute,
		AttributeMetadataCacheMaxSize: 1000,
	})

	attrs := pcommon.NewMap()
	attrs.PutStr("http.method", "GET")

	// First call should query ClickHouse
	metadata, err := reader.getAttributeMetadata(t.Context(), attrs)
	require.NoError(t, err)
	assert.Len(t, metadata, 1)
	assert.Len(t, d.RecordedQueries, 1)

	// Second call should use cache — no additional queries
	metadata, err = reader.getAttributeMetadata(t.Context(), attrs)
	require.NoError(t, err)
	assert.Len(t, metadata, 1)
	assert.Len(t, d.RecordedQueries, 1, "expected no additional queries due to cache hit")
}

func TestGetAttributeMetadata_CacheTTLExpiration(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		d := makeTestDriverWithMetadata(t)
		cacheTTL := 5 * time.Minute
		reader := NewReader(d, ReaderConfig{
			AttributeMetadataCacheTTL:     cacheTTL,
			AttributeMetadataCacheMaxSize: 1000,
		})

		attrs := pcommon.NewMap()
		attrs.PutStr("http.method", "GET")

		// First call populates cache
		metadata, err := reader.getAttributeMetadata(t.Context(), attrs)
		require.NoError(t, err)
		assert.Len(t, metadata, 1)
		assert.Len(t, d.RecordedQueries, 1)

		// Second call within TTL should use cache
		metadata, err = reader.getAttributeMetadata(t.Context(), attrs)
		require.NoError(t, err)
		assert.Len(t, metadata, 1)
		assert.Len(t, d.RecordedQueries, 1)

		// Advance time past TTL
		time.Sleep(cacheTTL + time.Second)

		// Reset row iterator so the re-query can scan rows again
		resp := d.QueryResponses[sql.SelectAttributeMetadata]
		rows := resp.Rows.(*clickhousetest.Rows[dbmodel.AttributeMetadata])
		rows.Index = 0

		// Call after TTL expiration should query ClickHouse again
		metadata, err = reader.getAttributeMetadata(t.Context(), attrs)
		require.NoError(t, err)
		assert.Len(t, metadata, 1)
		assert.Len(t, d.RecordedQueries, 2, "expected re-query after cache TTL expired")
	})
}

func TestGetAttributeMetadata_DoesNotCacheEmptyResult(t *testing.T) {
	// When metadata returns no rows for a key, the empty result should NOT be cached.
	d := &clickhousetest.Driver{
		T: t,
		QueryResponses: map[string]*clickhousetest.QueryResponse{
			sql.SelectAttributeMetadata: {
				Rows: &clickhousetest.Rows[dbmodel.AttributeMetadata]{
					Data: []dbmodel.AttributeMetadata{}, // no results
				},
			},
		},
	}
	reader := NewReader(d, ReaderConfig{
		AttributeMetadataCacheTTL:     time.Minute,
		AttributeMetadataCacheMaxSize: 1000,
	})

	attrs := pcommon.NewMap()
	attrs.PutStr("nonexistent.key", "value")

	// First call queries ClickHouse, gets no results
	metadata, err := reader.getAttributeMetadata(t.Context(), attrs)
	require.NoError(t, err)
	assert.Empty(t, metadata["nonexistent.key"].span)
	assert.Len(t, d.RecordedQueries, 1)

	// Second call should query ClickHouse again since empty results are not cached
	metadata, err = reader.getAttributeMetadata(t.Context(), attrs)
	require.NoError(t, err)
	assert.Empty(t, metadata["nonexistent.key"].span)
	assert.Len(t, d.RecordedQueries, 2, "expected another query since empty results are not cached")
}

func TestGetAttributeMetadata_NonStringAttributesSkipped(t *testing.T) {
	d := makeTestDriverWithMetadata(t)
	reader := NewReader(d, ReaderConfig{
		AttributeMetadataCacheTTL:     time.Minute,
		AttributeMetadataCacheMaxSize: 1000,
	})

	attrs := pcommon.NewMap()
	attrs.PutStr("http.method", "GET")
	attrs.PutBool("some.bool", true)
	attrs.PutInt("some.int", 42)
	attrs.PutDouble("some.double", 3.14)

	metadata, err := reader.getAttributeMetadata(t.Context(), attrs)
	require.NoError(t, err)
	assert.Len(t, metadata, 1, "only the string attribute should be in metadata")
	assert.Contains(t, metadata, "http.method")
	assert.NotContains(t, metadata, "some.bool")
	assert.NotContains(t, metadata, "some.int")
	assert.NotContains(t, metadata, "some.double")
}
