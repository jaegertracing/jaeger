// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package dbmodel

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaegertracing/jaeger-idl/model/v1"
)

func TestGetIntersectedTraceIDs(t *testing.T) {
	firstTraceID := TraceIDFromDomain(model.NewTraceID(1, 1))
	secondTraceID := TraceIDFromDomain(model.NewTraceID(2, 2))
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
	assert.EqualValues(t, expected, actual)
}

func TestAdd(t *testing.T) {
	u := UniqueTraceIDs{}
	someID := TraceIDFromDomain(model.NewTraceID(1, 1))
	u.Add(someID)
	assert.Contains(t, u, someID)
}

func TestFromList(t *testing.T) {
	someID := TraceIDFromDomain(model.NewTraceID(1, 1))
	traceIDList := []TraceID{
		someID,
	}
	uniqueTraceIDs := UniqueTraceIDsFromList(traceIDList)
	assert.Len(t, uniqueTraceIDs, 1)
	assert.Contains(t, uniqueTraceIDs, someID)
}
