// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package dbmodel

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFromDBModel(t *testing.T) {
	dbTrace := jsonToDBModel(t, "./fixtures/dbmodel.json")
	expected := jsonToPtrace(t, "./fixtures/ptrace.json")
	actual := FromDBModel(dbTrace)

	require.Equal(t, expected.ResourceSpans().Len(), actual.ResourceSpans().Len(), "ResourceSpans count mismatch")
	if actual.ResourceSpans().Len() == 0 {
		t.Fatal("Actual trace contains no ResourceSpans")
	}
	expectedResourceSpans := expected.ResourceSpans().At(0)
	actualResourceSpans := actual.ResourceSpans().At(0)

	require.Equal(t, expectedResourceSpans.ScopeSpans().Len(), actualResourceSpans.ScopeSpans().Len(), "ScopeSpans count mismatch")
	if actualResourceSpans.ScopeSpans().Len() == 0 {
		t.Fatal("Actual ResourceSpans contains no ScopeSpans")
	}
	exceptedScopeSpans := expectedResourceSpans.ScopeSpans().At(0)
	actualScopeSpans := actualResourceSpans.ScopeSpans().At(0)

	require.Equal(t, exceptedScopeSpans.Spans().Len(), actualScopeSpans.Spans().Len(), "Spans count mismatch")
	if actualScopeSpans.Spans().Len() == 0 {
		t.Fatal("Actual ScopeSpans contains no Spans")
	}
	exceptedSpan := exceptedScopeSpans.Spans().At(0)
	actualSpan := actualScopeSpans.Spans().At(0)

	t.Run("Resource Attributes", func(t *testing.T) {
		expectedResourceAttr := expectedResourceSpans.Resource().Attributes().AsRaw()
		actualResourceAttr := actualResourceSpans.Resource().Attributes().AsRaw()
		require.Equal(t, expectedResourceAttr, actualResourceAttr, "Resource attributes mismatch")
	})

	t.Run("Scope Attributes", func(t *testing.T) {
		exceptedScopeAttr := exceptedScopeSpans.Scope().Attributes().AsRaw()
		actualScopeAttr := actualScopeSpans.Scope().Attributes().AsRaw()
		require.Equal(t, exceptedScopeAttr, actualScopeAttr, "Scope attributes mismatch")
	})

	t.Run("Span Attributes", func(t *testing.T) {
		exceptedSpanAttr := exceptedSpan.Attributes().AsRaw()
		actualSpanAttr := actualSpan.Attributes().AsRaw()
		require.Equal(t, exceptedSpanAttr, actualSpanAttr, "Span attributes mismatch")
	})

	t.Run("Events Attributes", func(t *testing.T) {
		exceptedEvents := exceptedSpan.Events()
		actualEvents := actualSpan.Events()
		require.Equal(t, exceptedEvents.Len(), actualEvents.Len(), "Events count mismatch")
		for i := 0; i < exceptedEvents.Len(); i++ {
			exceptedEvent := exceptedEvents.At(i)
			exceptedEventAttr := exceptedEvent.Attributes().AsRaw()
			actualEvent := actualEvents.At(i)
			actualEventAttr := actualEvent.Attributes().AsRaw()
			require.Equal(t, exceptedEventAttr, actualEventAttr, "Event %d attributes mismatch", i)
		}
	})

	t.Run("Links Attributes", func(t *testing.T) {
		exceptedLinks := exceptedSpan.Links()
		actualLinks := actualSpan.Links()
		require.Equal(t, exceptedLinks.Len(), actualLinks.Len(), "Links count mismatch")
		for i := 0; i < exceptedLinks.Len(); i++ {
			exceptedLink := exceptedLinks.At(i)
			exceptedLinkAttr := exceptedLink.Attributes().AsRaw()
			actualLink := actualLinks.At(i)
			actualLinkAttr := actualLink.Attributes().AsRaw()
			require.Equal(t, exceptedLinkAttr, actualLinkAttr, "Link %d attributes mismatch", i)
		}
	})
}

func jsonToDBModel(t *testing.T, filename string) (m Trace) {
	traceBytes := readJSONBytes(t, filename)
	err := json.Unmarshal(traceBytes, &m)
	require.NoError(t, err, "Failed to read file %s", filename)
	return m
}
