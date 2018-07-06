// Copyright (c) 2017 Uber Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package dbmodel

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaegertracing/jaeger/model"
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
