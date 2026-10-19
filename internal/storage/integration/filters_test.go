// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"

	builder "github.com/jaegertracing/jaeger/internal/expression"
	"github.com/jaegertracing/jaeger/internal/jptrace"
)

func TestFilterCaseExpectations(t *testing.T) {
	entries, err := fixtures.ReadDir(filterCorpusDir)
	require.NoError(t, err)
	fixtureNames := make(map[string]struct{}, len(entries))
	traceOwners := make(map[pcommon.TraceID]string)
	for _, entry := range entries {
		name := strings.TrimSuffix(entry.Name(), ".json")
		fixtureNames[name] = struct{}{}
		trace := loadOTLPTrace(t, filterCorpusDir+"/"+entry.Name())
		for _, span := range jptrace.SpanIter(trace) {
			owner, exists := traceOwners[span.TraceID()]
			require.True(t, !exists || owner == name, "fixtures %q and %q share trace ID %s", owner, name, span.TraceID())
			traceOwners[span.TraceID()] = name
		}
	}

	// Case variations of the same search must return identical results.
	allowedIdenticalExpectations := map[[2]string]string{
		{"a phrase matches adjacent ordered words", "a phrase is case insensitive"}: "case-insensitive phrase search",
		{"fulltext matches all words in any order", "fulltext is case insensitive"}: "case-insensitive fulltext search",
	}
	// These negative cases deliberately assert that no trace matches.
	allowedEmptyExpectations := map[string]struct{}{
		"a phrase does not match when words are not adjacent":       {},
		"a phrase does not match when words are in the wrong order": {},
		"fulltext does not match when a word is missing":            {},
	}
	usedAllowances := make(map[[2]string]struct{})
	usedEmptyAllowances := make(map[string]struct{})
	seen := make(map[string]string)
	var p builder.Predicate
	for _, testCase := range filterTestCases(p) {
		names := slices.Clone(testCase.expected)
		slices.Sort(names)
		if len(names) == 0 {
			_, allowed := allowedEmptyExpectations[testCase.caption]
			require.True(t, allowed, "%q has no matching fixture", testCase.caption)
			usedEmptyAllowances[testCase.caption] = struct{}{}
			continue
		}
		require.Less(t, len(names), len(fixtureNames), "%q has no nonmatching fixture", testCase.caption)
		for _, name := range names {
			require.Contains(t, fixtureNames, name, "%q names an unknown fixture", testCase.caption)
		}
		require.Len(t, slices.Compact(slices.Clone(names)), len(names), "%q repeats a fixture", testCase.caption)

		key := strings.Join(names, "\x00")
		if previous, ok := seen[key]; ok {
			pair := [2]string{previous, testCase.caption}
			reason, allowed := allowedIdenticalExpectations[pair]
			require.True(t, allowed, "%q and %q expect the same fixtures: %v", previous, testCase.caption, names)
			require.NotEmpty(t, reason, "%q and %q need a reason for identical expectations", previous, testCase.caption)
			usedAllowances[pair] = struct{}{}
		} else {
			seen[key] = testCase.caption
		}
	}
	require.Len(t, usedAllowances, len(allowedIdenticalExpectations), "remove stale identical-expectation allowances")
	require.Len(t, usedEmptyAllowances, len(allowedEmptyExpectations), "remove stale empty-expectation allowances")
}
