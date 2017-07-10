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

package integration

import (
	"encoding/json"
	"testing"

	"github.com/kr/pretty"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/uber/jaeger/model"
)

func CompareSliceOfTraces(t *testing.T, expected []*model.Trace, actual []*model.Trace) {
	require.Equal(t, len(expected), len(actual))
	model.SortTraces(expected)
	model.SortTraces(actual)
	for i := range expected {
		checkSize(t, expected[i], actual[i])
	}
	if !assert.EqualValues(t, expected, actual) {
		for _, err := range pretty.Diff(expected, actual) {
			t.Log(err)
		}
		out, err := json.Marshal(actual)
		assert.NoError(t, err)
		t.Logf("Actual traces: %s", string(out))
	}
}

func CompareTraces(t *testing.T, expected *model.Trace, actual *model.Trace) {
	if expected.Spans == nil {
		require.Nil(t, actual.Spans)
		return
	}
	model.SortTrace(expected)
	model.SortTrace(actual)
	checkSize(t, expected, actual)
	if !assert.EqualValues(t, expected, actual) {
		for _, err := range pretty.Diff(expected, actual) {
			t.Log(err)
		}
		out, err := json.Marshal(actual)
		assert.NoError(t, err)
		t.Logf("Actual trace: %s", string(out))
	}
}

func checkSize(t *testing.T, expected *model.Trace, actual *model.Trace) {
	require.True(t, len(expected.Spans) == len(actual.Spans))
	for i := range expected.Spans {
		expectedSpan := expected.Spans[i]
		actualSpan := actual.Spans[i]
		require.True(t, len(expectedSpan.Tags) == len(actualSpan.Tags))
		require.True(t, len(expectedSpan.Logs) == len(actualSpan.Logs))
		if expectedSpan.Process != nil && actualSpan.Process != nil {
			require.True(t, len(expectedSpan.Process.Tags) == len(actualSpan.Process.Tags))
		}
	}
}
