// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package dbmodel

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

var (
	firstTraceID  = TraceID{0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 1}
	secondTraceID = TraceID{0, 0, 0, 0, 0, 0, 0, 2, 0, 0, 0, 0, 0, 0, 0, 2}

	// benchSink prevents the compiler from optimizing away benchmark results.
	benchSink UniqueTraceIDs
)

func TestGetIntersectedTraceIDs(t *testing.T) {
	listOfUniqueTraceIDs := []UniqueTraceIDs{
		{
			firstTraceID:  struct{}{},
			secondTraceID: struct{}{},
		},
		{
			firstTraceID:  struct{}{},
			secondTraceID: struct{}{},
		},
		{
			firstTraceID: struct{}{},
		},
	}
	expected := UniqueTraceIDs{
		firstTraceID: struct{}{},
	}
	actual := IntersectTraceIDs(listOfUniqueTraceIDs)
	assert.Equal(t, expected, actual)
}

func TestAdd(t *testing.T) {
	u := UniqueTraceIDs{}
	someID := firstTraceID
	u.Add(someID)
	assert.Contains(t, u, someID)
}

func TestFromList(t *testing.T) {
	someID := firstTraceID
	traceIDList := []TraceID{
		someID,
	}
	uniqueTraceIDs := UniqueTraceIDsFromList(traceIDList)
	assert.Len(t, uniqueTraceIDs, 1)
	assert.Contains(t, uniqueTraceIDs, someID)
}

func TestIntersectTraceIDs_SmallestSetNotFirst(t *testing.T) {
	// The smallest set is the last element; verify the result is
	// identical regardless of ordering.
	large := UniqueTraceIDs{}
	for i := range 1000 {
		var id TraceID
		id[15] = byte(i)
		id[14] = byte(i >> 8)
		large[id] = struct{}{}
	}
	// Build a shared ID that is guaranteed to be in the large set.
	var sharedID TraceID
	sharedID[15] = byte(42)
	small := UniqueTraceIDs{
		sharedID: {},
	}
	actual := IntersectTraceIDs([]UniqueTraceIDs{large, small})
	expected := UniqueTraceIDs{
		sharedID: {},
	}
	assert.Equal(t, expected, actual)
}

func BenchmarkIntersectTraceIDs_Asymmetric(b *testing.B) {
	// Build two sets with asymmetric sizes: one large (10 000 IDs),
	// one small (10 IDs) that is a subset of the large set.
	large := make(UniqueTraceIDs, 10_000)
	for i := range 10_000 {
		var id TraceID
		id[15] = byte(i)
		id[14] = byte(i >> 8)
		large[id] = struct{}{}
	}
	small := make(UniqueTraceIDs, 10)
	i := 0
	for id := range large {
		if i >= 10 {
			break
		}
		small[id] = struct{}{}
		i++
	}

	// Place the large set first so the old code would iterate 10 000 entries,
	// while the optimized code iterates only 10.
	sets := []UniqueTraceIDs{large, small}

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		benchSink = IntersectTraceIDs(sets)
	}
}
