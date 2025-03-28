// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2018 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package spanstore

import (
	"encoding/json"
	"sort"
	"testing"

	"github.com/kr/pretty"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/dbmodel"
)

func CompareJSONSpans(t *testing.T, expected *dbmodel.Span, actual *dbmodel.Span) {
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

func sortJSONSpan(span *dbmodel.Span) {
	sortJSONTags(span.Tags)
	sortJSONLogs(span.Logs)
	sortJSONProcess(span.Process)
}

type JSONTagByKey []dbmodel.KeyValue

func (t JSONTagByKey) Len() int           { return len(t) }
func (t JSONTagByKey) Swap(i, j int)      { t[i], t[j] = t[j], t[i] }
func (t JSONTagByKey) Less(i, j int) bool { return t[i].Key < t[j].Key }

func sortJSONTags(tags []dbmodel.KeyValue) {
	sort.Sort(JSONTagByKey(tags))
}

type JSONLogByTimestamp []dbmodel.Log

func (t JSONLogByTimestamp) Len() int           { return len(t) }
func (t JSONLogByTimestamp) Swap(i, j int)      { t[i], t[j] = t[j], t[i] }
func (t JSONLogByTimestamp) Less(i, j int) bool { return t[i].Timestamp < t[j].Timestamp }

func sortJSONLogs(logs []dbmodel.Log) {
	sort.Sort(JSONLogByTimestamp(logs))
	for i := range logs {
		sortJSONTags(logs[i].Fields)
	}
}

func sortJSONProcess(process dbmodel.Process) {
	sortJSONTags(process.Tags)
}
