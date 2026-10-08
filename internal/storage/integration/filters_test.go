// Copyright (c) 2026 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	builder "github.com/jaegertracing/jaeger/internal/expression"
)

func TestFilterCaseExpectations(t *testing.T) {
	entries, err := fixtures.ReadDir(filterCorpusDir)
	require.NoError(t, err)
	fixtureNames := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		fixtureNames[strings.TrimSuffix(entry.Name(), ".json")] = struct{}{}
	}

	// Keep a reason beside any pair that must share an expected set. There are no such pairs
	// today; a new one should be a deliberate exception rather than a silent loss of coverage.
	allowedIdenticalExpectations := map[[2]string]string{}
	usedAllowances := make(map[[2]string]struct{})
	seen := make(map[string]string)
	var p builder.Predicate
	for _, testCase := range filterTestCases(p) {
		names := slices.Clone(testCase.expected)
		slices.Sort(names)
		require.NotEmpty(t, names, "%q has no matching fixture", testCase.caption)
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
}
