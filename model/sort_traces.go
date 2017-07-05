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

	"github.com/pkg/errors"
)

type spanBySpanID []*Span

func (s spanBySpanID) Len() int           { return len(s) }
func (s spanBySpanID) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
func (s spanBySpanID) Less(i, j int) bool { return s[i].SpanID < s[j].SpanID }

// SortTraces checks two traces' length and sorts their spans.
func SortTraces(expected *Trace, actual *Trace) error {
	expectedSpans := expected.Spans
	actualSpans := actual.Spans
	if len(expectedSpans) != len(actualSpans) {
		return errors.New("traces have different number of spans")
	}
	sort.Sort(spanBySpanID(expectedSpans))
	sort.Sort(spanBySpanID(actualSpans))
	for i := range expectedSpans {
		if err := sortSpans(expectedSpans[i], actualSpans[i]); err != nil {
			return err
		}
	}
	return nil
}

func sortSpans(expected *Span, actual *Span) error {
	expected.NormalizeTimestamps()
	actual.NormalizeTimestamps()
	if err := sortTags(expected.Tags, actual.Tags); err != nil {
		return err
	}
	if err := sortLogs(expected.Logs, actual.Logs); err != nil {
		return err
	}
	if err := sortProcess(expected.Process, actual.Process); err != nil {
		return err
	}
	return nil
}

type tagByKey []KeyValue

func (t tagByKey) Len() int           { return len(t) }
func (t tagByKey) Swap(i, j int)      { t[i], t[j] = t[j], t[i] }
func (t tagByKey) Less(i, j int) bool { return t[i].Key < t[j].Key }

func sortTags(expected []KeyValue, actual []KeyValue) error {
	if len(expected) != len(actual) {
		return errors.New("tags have different length")
	}
	sort.Sort(tagByKey(expected))
	sort.Sort(tagByKey(actual))
	return nil
}

type logByTimestamp []Log

func (t logByTimestamp) Len() int           { return len(t) }
func (t logByTimestamp) Swap(i, j int)      { t[i], t[j] = t[j], t[i] }
func (t logByTimestamp) Less(i, j int) bool { return t[i].Timestamp.Before(t[j].Timestamp) }

func sortLogs(expected []Log, actual []Log) error {
	if len(expected) != len(actual) {
		return errors.New("logs have different length")
	}
	sort.Sort(logByTimestamp(expected))
	sort.Sort(logByTimestamp(actual))
	for i := range expected {
		sortTags(expected[i].Fields, actual[i].Fields)
	}
	return nil
}

func sortProcess(expected *Process, actual *Process) error {
	if expected == nil || actual == nil {
		if expected == nil && actual == nil {
			return nil
		}
		return errors.New("process does not match")
	}
	return sortTags(expected.Tags, actual.Tags)
}
