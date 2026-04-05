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

	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/sql"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/tracestore/dbmodel"
)

func TestGetAttributeMetadata_ErrorCases(t *testing.T) {
	attrs := pcommon.NewMap()
	attrs.PutStr("http.method", "GET")

	tests := []struct {
		name        string
		driver      *testDriver
		expectedErr string
	}{
		{
			name: "QueryError",
			driver: &testDriver{
				t: t,
				queryResponses: map[string]*testQueryResponse{
					sql.SelectAttributeMetadata: {
						rows: nil,
						err:  assert.AnError,
					},
				},
			},
			expectedErr: "failed to query attribute metadata",
		},
		{
			name: "ScanStructError",
			driver: &testDriver{
				t: t,
				queryResponses: map[string]*testQueryResponse{
					sql.SelectAttributeMetadata: {
						rows: &testRows[dbmodel.AttributeMetadata]{
							data: []dbmodel.AttributeMetadata{{
								AttributeKey: "http.method",
								Type:         "str",
								Level:        "span",
							}},
							scanErr: assert.AnError,
						},
						err: nil,
					},
				},
			},
			expectedErr: "failed to scan row",
		},
		{
			name: "RowsIterationError",
			driver: &testDriver{
				t: t,
				queryResponses: map[string]*testQueryResponse{
					sql.SelectAttributeMetadata: {
						rows: &testRows[dbmodel.AttributeMetadata]{
							data: []dbmodel.AttributeMetadata{{
								AttributeKey: "http.method",
								Type:         "str",
								Level:        "span",
							}},
							scanFn: func(dest any, src dbmodel.AttributeMetadata) error {
								ptr, ok := dest.(*dbmodel.AttributeMetadata)
								if !ok {
									return assert.AnError
								}
								*ptr = src
								return nil
							},
							rowsErr: assert.AnError,
						},
						err: nil,
					},
				},
			},
			expectedErr: "error iterating attribute metadata rows",
		},
		{
			name: "UnknownLevelError",
			driver: &testDriver{
				t: t,
				queryResponses: map[string]*testQueryResponse{
					sql.SelectAttributeMetadata: {
						rows: &testRows[dbmodel.AttributeMetadata]{
							data: []dbmodel.AttributeMetadata{{
								AttributeKey: "http.method",
								Type:         "str",
								Level:        "unknown",
							}},
							scanFn: func(dest any, src dbmodel.AttributeMetadata) error {
								ptr, ok := dest.(*dbmodel.AttributeMetadata)
								if !ok {
									return assert.AnError
								}
								*ptr = src
								return nil
							},
						},
						err: nil,
					},
				},
			},
			expectedErr: "unknown attribute level",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := NewReader(tt.driver, ReaderConfig{})
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

	driver := &testDriver{
		t: t,
	}

	reader := NewReader(driver, ReaderConfig{})
	metadata, err := reader.getAttributeMetadata(t.Context(), attrs)
	require.NoError(t, err)
	assert.Empty(t, metadata)
	assert.Empty(t, driver.recordedQueries)
}

func makeTestDriverWithMetadata(t *testing.T) *testDriver {
	return &testDriver{
		t: t,
		queryResponses: map[string]*testQueryResponse{
			sql.SelectAttributeMetadata: {
				rows: &testRows[dbmodel.AttributeMetadata]{
					data: []dbmodel.AttributeMetadata{
						{AttributeKey: "http.method", Type: "str", Level: "span"},
					},
					scanFn: func(dest any, src dbmodel.AttributeMetadata) error {
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
		AttributeMetadataCacheTTL: time.Minute,
	})

	attrs := pcommon.NewMap()
	attrs.PutStr("http.method", "GET")

	// Key is not in cache, should query ClickHouse
	metadata, err := reader.getAttributeMetadata(t.Context(), attrs)
	require.NoError(t, err)
	assert.Len(t, metadata, 1)
	assert.Len(t, d.recordedQueries, 1, "expected query to ClickHouse on cache miss")
	assert.Equal(t, 1, reader.attrMetaCache.Size(), "result should be cached after miss")
}

func TestGetAttributeMetadata_CacheHit(t *testing.T) {
	d := makeTestDriverWithMetadata(t)
	reader := NewReader(d, ReaderConfig{
		AttributeMetadataCacheTTL: time.Minute,
	})

	attrs := pcommon.NewMap()
	attrs.PutStr("http.method", "GET")

	// First call should query ClickHouse
	metadata, err := reader.getAttributeMetadata(t.Context(), attrs)
	require.NoError(t, err)
	assert.Len(t, metadata, 1)
	assert.Len(t, d.recordedQueries, 1)

	// Second call should use cache — no additional queries
	metadata, err = reader.getAttributeMetadata(t.Context(), attrs)
	require.NoError(t, err)
	assert.Len(t, metadata, 1)
	assert.Len(t, d.recordedQueries, 1, "expected no additional queries due to cache hit")
}

func TestGetAttributeMetadata_CacheDisabled(t *testing.T) {
	d := makeTestDriverWithMetadata(t)
	reader := NewReader(d, ReaderConfig{
		AttributeMetadataCacheTTL: 0,
	})

	attrs := pcommon.NewMap()
	attrs.PutStr("http.method", "GET")

	// Each call should query ClickHouse
	_, err := reader.getAttributeMetadata(t.Context(), attrs)
	require.NoError(t, err)
	assert.Len(t, d.recordedQueries, 1)

	_, err = reader.getAttributeMetadata(t.Context(), attrs)
	require.NoError(t, err)
	assert.Len(t, d.recordedQueries, 2, "expected additional query since cache is disabled")
}

func TestGetAttributeMetadata_CacheTTLExpiration(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		d := makeTestDriverWithMetadata(t)
		cacheTTL := 5 * time.Minute
		reader := NewReader(d, ReaderConfig{
			AttributeMetadataCacheTTL: cacheTTL,
		})

		attrs := pcommon.NewMap()
		attrs.PutStr("http.method", "GET")

		// First call populates cache
		metadata, err := reader.getAttributeMetadata(t.Context(), attrs)
		require.NoError(t, err)
		assert.Len(t, metadata, 1)
		assert.Len(t, d.recordedQueries, 1)

		// Second call within TTL should use cache
		metadata, err = reader.getAttributeMetadata(t.Context(), attrs)
		require.NoError(t, err)
		assert.Len(t, metadata, 1)
		assert.Len(t, d.recordedQueries, 1)

		// Advance time past TTL
		time.Sleep(cacheTTL + time.Second)

		// Reset row iterator so the re-query can scan rows again
		resp := d.queryResponses[sql.SelectAttributeMetadata]
		rows := resp.rows.(*testRows[dbmodel.AttributeMetadata])
		rows.index = 0

		// Call after TTL expiration should query ClickHouse again
		metadata, err = reader.getAttributeMetadata(t.Context(), attrs)
		require.NoError(t, err)
		assert.Len(t, metadata, 1)
		assert.Len(t, d.recordedQueries, 2, "expected re-query after cache TTL expired")
	})
}

func TestGetAttributeMetadata_CacheEmptyResult(t *testing.T) {
	// When metadata returns no rows for a key, the empty result should be cached
	// so subsequent queries for the same key don't hit ClickHouse.
	d := &testDriver{
		t: t,
		queryResponses: map[string]*testQueryResponse{
			sql.SelectAttributeMetadata: {
				rows: &testRows[dbmodel.AttributeMetadata]{
					data: []dbmodel.AttributeMetadata{}, // no results
				},
			},
		},
	}
	reader := NewReader(d, ReaderConfig{
		AttributeMetadataCacheTTL: time.Minute,
	})

	attrs := pcommon.NewMap()
	attrs.PutStr("nonexistent.key", "value")

	// First call queries ClickHouse, gets no results
	metadata, err := reader.getAttributeMetadata(t.Context(), attrs)
	require.NoError(t, err)
	assert.Empty(t, metadata["nonexistent.key"].span)
	assert.Len(t, d.recordedQueries, 1)

	// Second call should use cached empty result
	metadata, err = reader.getAttributeMetadata(t.Context(), attrs)
	require.NoError(t, err)
	assert.Empty(t, metadata["nonexistent.key"].span)
	assert.Len(t, d.recordedQueries, 1, "expected no additional query for cached empty result")
}
