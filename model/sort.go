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

package model

import (
	"sort"
)

type traceByTraceID []*Trace

func (s traceByTraceID) Len() int      { return len(s) }
func (s traceByTraceID) Swap(i, j int) { s[i], s[j] = s[j], s[i] }
func (s traceByTraceID) Less(i, j int) bool {
	if len(s[i].Spans) == 0 {
		return true
	} else if len(s[j].Spans) == 0 {
		return false
	} else {
		return s[i].Spans[0].TraceID.Low < s[j].Spans[0].TraceID.Low
	}
}

// SortTraces deep sorts a list of traces by TraceID.
func SortTraces(traces []*Trace) {
	sort.Sort(traceByTraceID(traces))
	for _, trace := range traces {
		SortTrace(trace)
	}
}

type spanBySpanID []*Span

func (s spanBySpanID) Len() int           { return len(s) }
func (s spanBySpanID) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
func (s spanBySpanID) Less(i, j int) bool { return s[i].SpanID < s[j].SpanID }

// SortTrace deep sorts a trace's spans by SpanID.
func SortTrace(trace *Trace) {
	sort.Sort(spanBySpanID(trace.Spans))
	for _, span := range trace.Spans {
		SortSpan(span)
	}
}

// SortSpan deep sorts a span: this sorts its tags, logs by timestamp, tags in logs, and tags in process.
func SortSpan(span *Span) {
	span.NormalizeTimestamps()
	sortTags(span.Tags)
	sortLogs(span.Logs)
	sortProcess(span.Process)
}

type tagByKey []KeyValue

func (t tagByKey) Len() int           { return len(t) }
func (t tagByKey) Swap(i, j int)      { t[i], t[j] = t[j], t[i] }
func (t tagByKey) Less(i, j int) bool { return t[i].Key < t[j].Key }

func sortTags(tags []KeyValue) {
	sort.Sort(tagByKey(tags))
}

type logByTimestamp []Log

func (t logByTimestamp) Len() int           { return len(t) }
func (t logByTimestamp) Swap(i, j int)      { t[i], t[j] = t[j], t[i] }
func (t logByTimestamp) Less(i, j int) bool { return t[i].Timestamp.Before(t[j].Timestamp) }

func sortLogs(logs []Log) {
	sort.Sort(logByTimestamp(logs))
	for _, log := range logs {
		sortTags(log.Fields)
	}
}

func sortProcess(process *Process) {
	if process != nil {
		sortTags(process.Tags)
	}
}
