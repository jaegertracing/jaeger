// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package json

import (
	"encoding/json"
	"sort"
	"testing"

	"github.com/kr/pretty"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	esjson "github.com/jaegertracing/jaeger/internal/uimodel"
)

func CompareJSONSpans(t *testing.T, expected *esjson.Span, actual *esjson.Span) {
	sortJSONSpan(expected)
	sortJSONSpan(actual)

	if !assert.Equal(t, expected, actual) {
		for _, err := range pretty.Diff(expected, actual) {
			t.Log(err)
		}
		out, err := json.Marshal(actual)
		require.NoError(t, err)
		t.Logf("Actual trace: %s", string(out))
	}
}

func sortJSONSpan(span *esjson.Span) {
	sortJSONTags(span.Tags)
	sortJSONLogs(span.Logs)
	sortJSONProcess(span.Process)
}

type JSONTagByKey []esjson.KeyValue

func (t JSONTagByKey) Len() int           { return len(t) }
func (t JSONTagByKey) Swap(i, j int)      { t[i], t[j] = t[j], t[i] }
func (t JSONTagByKey) Less(i, j int) bool { return t[i].Key < t[j].Key }

func sortJSONTags(tags []esjson.KeyValue) {
	sort.Sort(JSONTagByKey(tags))
}

type JSONLogByTimestamp []esjson.Log

func (t JSONLogByTimestamp) Len() int           { return len(t) }
func (t JSONLogByTimestamp) Swap(i, j int)      { t[i], t[j] = t[j], t[i] }
func (t JSONLogByTimestamp) Less(i, j int) bool { return t[i].Timestamp < t[j].Timestamp }

func sortJSONLogs(logs []esjson.Log) {
	sort.Sort(JSONLogByTimestamp(logs))
	for i := range logs {
		sortJSONTags(logs[i].Fields)
	}
}

func sortJSONProcess(process *esjson.Process) {
	sortJSONTags(process.Tags)
}
