// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package dbmodel

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestToPtrace(t *testing.T) {
	dbTrace := jsonToDbTrace(t, "./fixtures/dbtrace.json")
	expected := jsonToTrace(t, "./fixtures/ptrace.json")
	actual := ToPtrace(dbTrace)

	exceptedResourceSpans := expected.ResourceSpans().At(0)
	expectedResourceAttr := exceptedResourceSpans.Resource().Attributes().AsRaw()
	actualResourceSpans := actual.ResourceSpans().At(0)
	actualResourceAttr := actualResourceSpans.Resource().Attributes().AsRaw()
	require.Equal(t, expectedResourceAttr, actualResourceAttr)

	exceptedScopeSpans := exceptedResourceSpans.ScopeSpans().At(0)
	exceptedScopeAttr := exceptedScopeSpans.Scope().Attributes().AsRaw()
	actualScopeSpans := actualResourceSpans.ScopeSpans().At(0)
	actualScopeAttr := actualScopeSpans.Scope().Attributes().AsRaw()
	require.Equal(t, exceptedScopeAttr, actualScopeAttr)

	exceptedSpan := exceptedScopeSpans.Spans().At(0)
	exceptedSpanAttr := exceptedSpan.Attributes().AsRaw()
	actualSpan := actualScopeSpans.Spans().At(0)
	actualSpanAttr := actualSpan.Attributes().AsRaw()
	require.Equal(t, exceptedSpanAttr, actualSpanAttr)

	exceptedEvents := exceptedSpan.Events()
	actualEvents := actualSpan.Events()
	require.Equal(t, exceptedEvents.Len(), actualEvents.Len())
	for i := 0; i < exceptedEvents.Len(); i++ {
		exceptedEvent := exceptedEvents.At(i)
		exceptedEventAttr := exceptedEvent.Attributes().AsRaw()
		actualEvent := actualEvents.At(i)
		actualEventAttr := actualEvent.Attributes().AsRaw()
		require.Equal(t, exceptedEventAttr, actualEventAttr)
	}

	exceptedLinks := exceptedSpan.Links()
	actualLinks := actualSpan.Links()
	require.Equal(t, exceptedLinks.Len(), actualLinks.Len())
	for i := 0; i < exceptedEvents.Len(); i++ {
		exceptedLink := exceptedEvents.At(i)
		exceptedLinkAttr := exceptedLink.Attributes().AsRaw()
		actualLink := actualEvents.At(i)
		actualLinkAttr := actualLink.Attributes().AsRaw()
		require.Equal(t, exceptedLinkAttr, actualLinkAttr)
	}
}

func jsonToDbTrace(t *testing.T, filename string) (dbTrace Trace) {
	traceBytes := readJSONBytes(t, filename)
	err := json.Unmarshal(traceBytes, &dbTrace)
	require.NoError(t, err, "Failed to read file %s", filename)
	return dbTrace
}
