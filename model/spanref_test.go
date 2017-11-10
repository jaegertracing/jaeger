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

package model_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaegertracing/jaeger/model"
)

func TestSpanRefTypeToFromString(t *testing.T) {
	badSpanRefType := model.SpanRefType(-1)
	testCases := []struct {
		v model.SpanRefType
		s string
	}{
		{model.ChildOf, "child-of"},
		{model.FollowsFrom, "follows-from"},
		{badSpanRefType, "<invalid>"},
	}
	for _, testCase := range testCases {
		assert.Equal(t, testCase.s, testCase.v.String(), testCase.s)
		v2, err := model.SpanRefTypeFromString(testCase.s)
		if testCase.v == badSpanRefType {
			assert.Error(t, err)
		} else {
			assert.NoError(t, err, testCase.s)
			assert.Equal(t, testCase.v, v2, testCase.s)
		}
	}
}

func TestSpanRefTypeToFromJSON(t *testing.T) {
	sr := model.SpanRef{
		RefType: model.ChildOf,
	}
	out, err := json.Marshal(sr)
	assert.NoError(t, err)
	assert.Equal(t, `{"refType":"child-of","traceID":"0","spanID":"0"}`, string(out))
	var sr2 model.SpanRef
	if assert.NoError(t, json.Unmarshal(out, &sr2)) {
		assert.Equal(t, sr, sr2)
	}
	var sr3 model.SpanRef
	err = json.Unmarshal([]byte(`{"refType":"BAD"}`), &sr3)
	assert.EqualError(t, err, "not a valid SpanRefType string BAD")
}
