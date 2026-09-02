// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package dbmodel

// UniqueTraceIDs is a set of unique dbmodel TraceIDs, implemented via map.
type UniqueTraceIDs map[TraceID]struct{}

// UniqueTraceIDsFromList Takes a list of traceIDs and returns the unique set
func UniqueTraceIDsFromList(traceIDs []TraceID) UniqueTraceIDs {
	uniqueTraceIDs := UniqueTraceIDs{}
	for _, traceID := range traceIDs {
		uniqueTraceIDs[traceID] = struct{}{}
	}
	return uniqueTraceIDs
}

// Add adds a traceID to the existing map
func (u UniqueTraceIDs) Add(traceID TraceID) {
	u[traceID] = struct{}{}
}

// IntersectTraceIDs takes a list of UniqueTraceIDs and intersects them.
func IntersectTraceIDs(uniqueTraceIdsList []UniqueTraceIDs) UniqueTraceIDs {
	// Find the smallest set to iterate over, as the intersection
	// can never be larger than the smallest input set.
	smallestIdx := 0
	for i := range uniqueTraceIdsList {
		if len(uniqueTraceIdsList[i]) < len(uniqueTraceIdsList[smallestIdx]) { //nolint:gosec // G602 false positive: smallestIdx is always a valid index
			smallestIdx = i
		}
	}
	retMe := UniqueTraceIDs{}
	for key, value := range uniqueTraceIdsList[smallestIdx] {
		keyExistsInAll := true
		for i, otherTraceIds := range uniqueTraceIdsList {
			if i == smallestIdx {
				continue
			}
			if _, ok := otherTraceIds[key]; !ok {
				keyExistsInAll = false
				break
			}
		}
		if keyExistsInAll {
			retMe[key] = value
		}
	}
	return retMe
}
