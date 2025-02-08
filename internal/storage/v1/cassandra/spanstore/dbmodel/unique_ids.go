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
	retMe := UniqueTraceIDs{}
	for key, value := range uniqueTraceIdsList[0] {
		keyExistsInAll := true
		for _, otherTraceIds := range uniqueTraceIdsList[1:] {
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
