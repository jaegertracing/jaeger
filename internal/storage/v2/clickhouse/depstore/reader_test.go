// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package depstore

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/depstore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/clickhousetest"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/sql"
)

func scanString(dest any, src string) error {
	dests := dest.([]any)
	*dests[0].(*string) = src
	return nil
}

func TestGetDependencies(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name        string
		conn        *clickhousetest.Driver
		query       depstore.QueryParameters
		expected    []model.DependencyLink
		expectError string
	}{
		{
			name: "successfully returns dependencies from single snapshot",
			conn: &clickhousetest.Driver{
				QueryResponses: map[string]*clickhousetest.QueryResponse{
					sql.SelectDependencies: {
						Rows: &clickhousetest.Rows[string]{
							Data: []string{
								`[{"parent":"serviceA","child":"serviceB","callCount":42},{"parent":"serviceB","child":"serviceC","callCount":7}]`,
							},
							ScanFn: scanString,
						},
					},
				},
			},
			query: depstore.QueryParameters{
				StartTime: now.Add(-1 * time.Hour),
				EndTime:   now,
			},
			expected: []model.DependencyLink{
				{Parent: "serviceA", Child: "serviceB", CallCount: 42},
				{Parent: "serviceB", Child: "serviceC", CallCount: 7},
			},
		},
		{
			name: "merges dependencies across multiple snapshots",
			conn: &clickhousetest.Driver{
				QueryResponses: map[string]*clickhousetest.QueryResponse{
					sql.SelectDependencies: {
						Rows: &clickhousetest.Rows[string]{
							Data: []string{
								`[{"parent":"serviceA","child":"serviceB","callCount":10}]`,
								`[{"parent":"serviceA","child":"serviceB","callCount":5},{"parent":"serviceC","child":"serviceD","callCount":3}]`,
							},
							ScanFn: scanString,
						},
					},
				},
			},
			query: depstore.QueryParameters{
				StartTime: now.Add(-2 * time.Hour),
				EndTime:   now,
			},
			expected: []model.DependencyLink{
				{Parent: "serviceA", Child: "serviceB", CallCount: 15},
				{Parent: "serviceC", Child: "serviceD", CallCount: 3},
			},
		},
		{
			name: "returns nil when no dependencies",
			conn: &clickhousetest.Driver{
				QueryResponses: map[string]*clickhousetest.QueryResponse{
					sql.SelectDependencies: {
						Rows: &clickhousetest.Rows[string]{},
					},
				},
			},
			query: depstore.QueryParameters{
				StartTime: now.Add(-1 * time.Hour),
				EndTime:   now,
			},
			expected: nil,
		},
		{
			name: "query error",
			conn: &clickhousetest.Driver{
				QueryResponses: map[string]*clickhousetest.QueryResponse{
					sql.SelectDependencies: {
						Err: assert.AnError,
					},
				},
			},
			query: depstore.QueryParameters{
				StartTime: now.Add(-1 * time.Hour),
				EndTime:   now,
			},
			expectError: "failed to query dependencies",
		},
		{
			name: "scan error",
			conn: &clickhousetest.Driver{
				QueryResponses: map[string]*clickhousetest.QueryResponse{
					sql.SelectDependencies: {
						Rows: &clickhousetest.Rows[string]{
							Data: []string{
								`[{"parent":"a","child":"b","callCount":1}]`,
							},
							ScanErr: assert.AnError,
						},
					},
				},
			},
			query: depstore.QueryParameters{
				StartTime: now.Add(-1 * time.Hour),
				EndTime:   now,
			},
			expectError: "failed to scan dependency row",
		},
		{
			name: "invalid JSON blob",
			conn: &clickhousetest.Driver{
				QueryResponses: map[string]*clickhousetest.QueryResponse{
					sql.SelectDependencies: {
						Rows: &clickhousetest.Rows[string]{
							Data: []string{
								`not valid json`,
							},
							ScanFn: scanString,
						},
					},
				},
			},
			query: depstore.QueryParameters{
				StartTime: now.Add(-1 * time.Hour),
				EndTime:   now,
			},
			expectError: "failed to unmarshal dependencies JSON",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reader := NewDependencyReader(test.conn)
			result, err := reader.GetDependencies(context.Background(), test.query)
			if test.expectError != "" {
				require.ErrorContains(t, err, test.expectError)
			} else {
				require.NoError(t, err)
				require.ElementsMatch(t, test.expected, result)
			}
		})
	}
}
