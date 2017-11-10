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

package integration

import (
	"encoding/json"
	"testing"

	"github.com/kr/pretty"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/model"
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
