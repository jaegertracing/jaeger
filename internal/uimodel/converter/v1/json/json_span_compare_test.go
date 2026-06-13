// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package json

import (
	"cmp"
	"encoding/json"
	"slices"
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

func sortJSONTags(tags []esjson.KeyValue) {
	slices.SortFunc(tags, func(a, b esjson.KeyValue) int {
		return cmp.Compare(a.Key, b.Key)
	})
}

func sortJSONLogs(logs []esjson.Log) {
	slices.SortFunc(logs, func(a, b esjson.Log) int {
		return cmp.Compare(a.Timestamp, b.Timestamp)
	})
	for i := range logs {
		sortJSONTags(logs[i].Fields)
	}
}

func sortJSONProcess(process *esjson.Process) {
	sortJSONTags(process.Tags)
}
