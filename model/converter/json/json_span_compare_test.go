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

package json

import (
	"encoding/json"
	"sort"
	"testing"

	"github.com/kr/pretty"
	"github.com/stretchr/testify/assert"

	esJson "github.com/uber/jaeger/model/json"
)

func CompareJSONSpans(t *testing.T, expected *esJson.Span, actual *esJson.Span) {
	sortJSONSpan(expected)
	sortJSONSpan(actual)

	if !assert.EqualValues(t, expected, actual) {
		for _, err := range pretty.Diff(expected, actual) {
			t.Log(err)
		}
		out, err := json.Marshal(actual)
		assert.NoError(t, err)
		t.Logf("Actual trace: %s", string(out))
	}
}

func sortJSONSpan(span *esJson.Span) {
	sortJSONTags(span.Tags)
	sortJSONLogs(span.Logs)
	sortJSONProcess(span.Process)
}

type JSONTagByKey []esJson.KeyValue

func (t JSONTagByKey) Len() int           { return len(t) }
func (t JSONTagByKey) Swap(i, j int)      { t[i], t[j] = t[j], t[i] }
func (t JSONTagByKey) Less(i, j int) bool { return t[i].Key < t[j].Key }

func sortJSONTags(tags []esJson.KeyValue) {
	sort.Sort(JSONTagByKey(tags))
}

type JSONLogByTimestamp []esJson.Log

func (t JSONLogByTimestamp) Len() int           { return len(t) }
func (t JSONLogByTimestamp) Swap(i, j int)      { t[i], t[j] = t[j], t[i] }
func (t JSONLogByTimestamp) Less(i, j int) bool { return t[i].Timestamp < t[j].Timestamp }

func sortJSONLogs(expected []esJson.Log) {
	sort.Sort(JSONLogByTimestamp(expected))
	for i := range expected {
		sortJSONTags(expected[i].Fields)
	}
}

func sortJSONProcess(expected *esJson.Process) {
	sortJSONTags(expected.Tags)
}
