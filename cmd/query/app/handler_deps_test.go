// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"sort"
	"testing"
	"time"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	ui "github.com/jaegertracing/jaeger/model/json"
	"github.com/jaegertracing/jaeger/storage_v2/depstore"
)

func TestDeduplicateDependencies(t *testing.T) {
	handler := &APIHandler{}
	tests := []struct {
		description string
		input       []model.DependencyLink
		expected    []ui.DependencyLink
	}{
		{
			"Single parent and child",
			[]model.DependencyLink{
				{
					Parent:    "Drogo",
					Child:     "Frodo",
					CallCount: 20,
				},
			},
			[]ui.DependencyLink{
				{
					Parent:    "Drogo",
					Child:     "Frodo",
					CallCount: 20,
				},
			},
		},
		{
			"Single parent, multiple children",
			[]model.DependencyLink{
				{
					Parent:    "Dáin I",
					Child:     "Thrór",
					CallCount: 314,
				},
				{
					Parent:    "Dáin I",
					Child:     "Frór",
					CallCount: 159,
				},
				{
					Parent:    "Dáin I",
					Child:     "Grór",
					CallCount: 265,
				},
			},
			[]ui.DependencyLink{
				{
					Parent:    "Dáin I",
					Child:     "Thrór",
					CallCount: 314,
				},
				{
					Parent:    "Dáin I",
					Child:     "Frór",
					CallCount: 159,
				},
				{
					Parent:    "Dáin I",
					Child:     "Grór",
					CallCount: 265,
				},
			},
		},
		{
			"multiple parents, single child",
			[]model.DependencyLink{
				{
					Parent:    "Hador",
					Child:     "Glóredhel",
					CallCount: 3,
				},
				{
					Parent:    "Gildis",
					Child:     "Glóredhel",
					CallCount: 9,
				},
			},
			[]ui.DependencyLink{
				{
					Parent:    "Hador",
					Child:     "Glóredhel",
					CallCount: 3,
				},
				{
					Parent:    "Gildis",
					Child:     "Glóredhel",
					CallCount: 9,
				},
			},
		},
		{
			"single parent, multiple children with duplicates",
			[]model.DependencyLink{
				{
					Parent:    "Dáin I",
					Child:     "Thrór",
					CallCount: 314,
				},
				{
					Parent:    "Dáin I",
					Child:     "Thrór",
					CallCount: 159,
				},
				{
					Parent:    "Dáin I",
					Child:     "Grór",
					CallCount: 265,
				},
			},
			[]ui.DependencyLink{
				{
					Parent:    "Dáin I",
					Child:     "Thrór",
					CallCount: 473,
				},
				{
					Parent:    "Dáin I",
					Child:     "Grór",
					CallCount: 265,
				},
			},
		},
	}

	for _, test := range tests {
		actual := handler.deduplicateDependencies(test.input)
		sort.Sort(DependencyLinks(actual))
		expected := test.expected
		sort.Sort(DependencyLinks(expected))
		assert.Equal(t, expected, actual, test.description)
	}
}

type DependencyLinks []ui.DependencyLink

func (slice DependencyLinks) Len() int {
	return len(slice)
}

func (slice DependencyLinks) Less(i, j int) bool {
	if slice[i].Parent != slice[j].Parent {
		return slice[i].Parent < slice[j].Parent
	}
	if slice[i].Child != slice[j].Child {
		return slice[i].Child < slice[j].Child
	}
	return slice[i].CallCount < slice[j].CallCount
}

func (slice DependencyLinks) Swap(i, j int) {
	slice[i], slice[j] = slice[j], slice[i]
}

func TestFilterDependencies(t *testing.T) {
	handler := &APIHandler{}
	tests := []struct {
		description  string
		service      string
		dependencies []model.DependencyLink
		expected     []model.DependencyLink
	}{
		{
			"No services filtered for %s",
			"Drogo",
			[]model.DependencyLink{
				{
					Parent:    "Drogo",
					Child:     "Frodo",
					CallCount: 20,
				},
			},
			[]model.DependencyLink{
				{
					Parent:    "Drogo",
					Child:     "Frodo",
					CallCount: 20,
				},
			},
		},
		{
			"No services filtered for empty string",
			"",
			[]model.DependencyLink{
				{
					Parent:    "Drogo",
					Child:     "Frodo",
					CallCount: 20,
				},
			},
			[]model.DependencyLink{
				{
					Parent:    "Drogo",
					Child:     "Frodo",
					CallCount: 20,
				},
			},
		},
		{
			"All services filtered away for %s",
			"Dáin I",
			[]model.DependencyLink{
				{
					Parent:    "Drogo",
					Child:     "Frodo",
					CallCount: 20,
				},
			},
			[]model.DependencyLink(nil),
		},
		{
			"Filter by parent %s",
			"Dáin I",
			[]model.DependencyLink{
				{
					Parent:    "Dáin I",
					Child:     "Thrór",
					CallCount: 314,
				},
				{
					Parent:    "Dáin I",
					Child:     "Frór",
					CallCount: 159,
				},
				{
					Parent:    "Dáin I",
					Child:     "Grór",
					CallCount: 265,
				},
			},
			[]model.DependencyLink{
				{
					Parent:    "Dáin I",
					Child:     "Thrór",
					CallCount: 314,
				},
				{
					Parent:    "Dáin I",
					Child:     "Frór",
					CallCount: 159,
				},
				{
					Parent:    "Dáin I",
					Child:     "Grór",
					CallCount: 265,
				},
			},
		},
		{
			"Filter by child %s",
			"Frór",
			[]model.DependencyLink{
				{
					Parent:    "Dáin I",
					Child:     "Thrór",
					CallCount: 314,
				},
				{
					Parent:    "Dáin I",
					Child:     "Frór",
					CallCount: 159,
				},
				{
					Parent:    "Dáin I",
					Child:     "Grór",
					CallCount: 265,
				},
			},
			[]model.DependencyLink{
				{
					Parent:    "Dáin I",
					Child:     "Frór",
					CallCount: 159,
				},
			},
		},
	}

	for _, test := range tests {
		actual := handler.filterDependenciesByService(test.dependencies, test.service)
		assert.Equal(t, test.expected, actual, test.description, test.service)
	}
}

func TestGetDependenciesSuccess(t *testing.T) {
	ts := initializeTestServer(t)
	expectedDependencies := []model.DependencyLink{{Parent: "killer", Child: "queen", CallCount: 12}}
	endTs := time.Unix(0, 1476374248550*millisToNanosMultiplier)
	ts.dependencyReader.On("GetDependencies",
		mock.Anything, // context
		depstore.QueryParameters{
			StartTime: endTs.Add(-defaultDependencyLookbackDuration),
			EndTime:   endTs,
		},
	).Return(expectedDependencies, nil).Times(1)

	var response structuredResponse
	err := getJSON(ts.server.URL+"/api/dependencies?endTs=1476374248550&service=queen", &response)
	assert.NotEmpty(t, response.Data)
	data := response.Data.([]any)[0]
	actual := data.(map[string]any)
	assert.Equal(t, "killer", actual["parent"])
	assert.Equal(t, "queen", actual["child"])
	assert.InDelta(t, 12.00, actual["callCount"], 0.01) // recovered type is float
	require.NoError(t, err)
}

func TestGetDependenciesCassandraFailure(t *testing.T) {
	ts := initializeTestServer(t)
	endTs := time.Unix(0, 1476374248550*millisToNanosMultiplier)
	ts.dependencyReader.On("GetDependencies",
		mock.Anything, // context
		depstore.QueryParameters{
			StartTime: endTs.Add(-defaultDependencyLookbackDuration),
			EndTime:   endTs,
		},
	).Return(nil, errStorage).Times(1)

	var response structuredResponse
	err := getJSON(ts.server.URL+"/api/dependencies?endTs=1476374248550&service=testing", &response)
	require.Error(t, err)
}

func TestGetDependenciesEndTimeParsingFailure(t *testing.T) {
	ts := initializeTestServer(t)
	var response structuredResponse
	err := getJSON(ts.server.URL+"/api/dependencies?endTs=shazbot&service=testing", &response)
	require.Error(t, err)
}

func TestGetDependenciesLookbackParsingFailure(t *testing.T) {
	ts := initializeTestServer(t)
	var response structuredResponse
	err := getJSON(ts.server.URL+"/api/dependencies?endTs=1476374248550&service=testing&lookback=shazbot", &response)
	require.Error(t, err)
}
