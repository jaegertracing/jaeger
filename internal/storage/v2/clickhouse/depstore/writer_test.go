// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package depstore

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/clickhousetest"
	"github.com/jaegertracing/jaeger/internal/storage/v2/clickhouse/sql"
)

var testDeps = []model.DependencyLink{
	{Parent: "serviceA", Child: "serviceB", CallCount: 10},
	{Parent: "serviceB", Child: "serviceC", CallCount: 5},
}

func TestWriteDependencies(t *testing.T) {
	ts := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	b := &clickhousetest.Batch{}
	conn := &clickhousetest.Driver{
		BatchResponses: map[string]*clickhousetest.BatchResponse{
			sql.InsertDependencies: {Batch: b},
		},
	}
	w := NewDependencyWriter(conn)
	err := w.WriteDependencies(t.Context(), ts, testDeps)
	require.NoError(t, err)

	require.True(t, b.SendCalled)
	require.Len(t, b.Appended, 1)
	row := b.Appended[0]
	assert.Equal(t, ts, row[0])
	expectedJSON := `[{"parent":"serviceA","child":"serviceB","callCount":10},{"parent":"serviceB","child":"serviceC","callCount":5}]`
	assert.JSONEq(t, expectedJSON, row[1].(string))
}

func TestWriteDependencies_Errors(t *testing.T) {
	tests := []struct {
		name        string
		batch       *clickhousetest.Batch
		batchErr    error
		expectError string
	}{
		{
			name:        "prepare batch error",
			batchErr:    assert.AnError,
			expectError: "failed to prepare batch",
		},
		{
			name:        "append error",
			batch:       &clickhousetest.Batch{AppendErr: assert.AnError},
			expectError: "failed to append to batch",
		},
		{
			name:        "send error",
			batch:       &clickhousetest.Batch{SendErr: assert.AnError},
			expectError: "failed to send batch",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			conn := &clickhousetest.Driver{
				BatchResponses: map[string]*clickhousetest.BatchResponse{
					sql.InsertDependencies: {Batch: test.batch, Err: test.batchErr},
				},
			}
			w := NewDependencyWriter(conn)
			err := w.WriteDependencies(t.Context(), time.Now(), testDeps)
			require.ErrorContains(t, err, test.expectError)
			require.ErrorIs(t, err, assert.AnError)
		})
	}
}
