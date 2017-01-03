// Copyright (c) 2017 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package model_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/uber/jaeger/model"
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
