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
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/uber/jaeger/model"
)

type TraceByTraceID []*model.Trace

func (s TraceByTraceID) Len() int      { return len(s) }
func (s TraceByTraceID) Swap(i, j int) { s[i], s[j] = s[j], s[i] }
func (s TraceByTraceID) Less(i, j int) bool {
	if len(s[i].Spans) != len(s[j].Spans) {
		return len(s[i].Spans) < len(s[j].Spans)
	} else if len(s[i].Spans) == 0 {
		return true
	}
	return s[i].Spans[0].TraceID.Low < s[j].Spans[0].TraceID.Low
}

func CompareListOfModelTraces(t *testing.T, expected []*model.Trace, actual []*model.Trace) {
	sort.Sort(TraceByTraceID(expected))
	sort.Sort(TraceByTraceID(actual))
	assert.Equal(t, len(expected), len(actual))
	for i := range expected {
		sortModelTraces(expected[i], actual[i])
	}
	assert.EqualValues(t, expected, actual)
}

type SpanBySpanID []*model.Span

func (s SpanBySpanID) Len() int           { return len(s) }
func (s SpanBySpanID) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
func (s SpanBySpanID) Less(i, j int) bool { return s[i].SpanID < s[j].SpanID }

func CompareModelTraces(t *testing.T, expected *model.Trace, actual *model.Trace) {
	if expected.Spans == nil {
		require.Nil(t, actual.Spans)
		return
	}
	sortModelTraces(expected, actual)
	assert.EqualValues(t, expected, actual)
}

func sortModelTraces(expected *model.Trace, actual *model.Trace) {
	expectedSpans := expected.Spans
	actualSpans := actual.Spans
	sort.Sort(SpanBySpanID(expectedSpans))
	sort.Sort(SpanBySpanID(actualSpans))
	for i := range expectedSpans {
		sortModelSpans(expectedSpans[i], actualSpans[i])
	}
}


func sortModelSpans(expected *model.Span, actual *model.Span) {
	sortModelTags(expected.Tags, actual.Tags)
	sortModelLogs(expected.Logs, actual.Logs)
	sortModelProcess(expected.Process, actual.Process)
}

type TagByKey []model.KeyValue

func (t TagByKey) Len() int           { return len(t) }
func (t TagByKey) Swap(i, j int)      { t[i], t[j] = t[j], t[i] }
func (t TagByKey) Less(i, j int) bool { return t[i].Key < t[j].Key }

func sortModelTags(expected []model.KeyValue, actual []model.KeyValue) {
	sort.Sort(TagByKey(expected))
	sort.Sort(TagByKey(actual))
}

type LogByTimestamp []model.Log

func (t LogByTimestamp) Len() int           { return len(t) }
func (t LogByTimestamp) Swap(i, j int)      { t[i], t[j] = t[j], t[i] }
func (t LogByTimestamp) Less(i, j int) bool { return t[i].Timestamp.Before(t[j].Timestamp) }

// this function exists solely to make it easier for developer to find out where the difference is
func sortModelLogs(expected []model.Log, actual []model.Log) {
	sort.Sort(LogByTimestamp(expected))
	sort.Sort(LogByTimestamp(actual))
	for i := range expected {
		sortModelTags(expected[i].Fields, actual[i].Fields)
	}
}

func sortModelProcess(expected *model.Process, actual *model.Process) {
	sortModelTags(expected.Tags, actual.Tags)
}
