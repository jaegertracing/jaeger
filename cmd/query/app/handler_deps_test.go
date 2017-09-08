// Copyright (c) 2017 Uber Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package app

import (
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/uber/jaeger/model"
	ui "github.com/uber/jaeger/model/json"
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
	server, _, mock := initializeTestServer()
	defer server.Close()
	expectedDependencies := []model.DependencyLink{{Parent: "killer", Child: "queen", CallCount: 12}}
	endTs := time.Unix(0, 1476374248550*millisToNanosMultiplier)
	mock.On("GetDependencies", endTs, defaultDependencyLookbackDuration).Return(expectedDependencies, nil).Times(1)

	var response structuredResponse
	err := getJSON(server.URL+"/api/dependencies?endTs=1476374248550&service=queen", &response)
	assert.NotEmpty(t, response.Data)
	data := response.Data.([]interface{})[0]
	actual := data.(map[string]interface{})
	assert.Equal(t, actual["parent"], "killer")
	assert.Equal(t, actual["child"], "queen")
	assert.Equal(t, actual["callCount"], 12.00) //recovered type is float
	assert.NoError(t, err)
}

func TestGetDependenciesCassandraFailure(t *testing.T) {
	server, _, mock := initializeTestServer()
	defer server.Close()
	endTs := time.Unix(0, 1476374248550*millisToNanosMultiplier)
	mock.On("GetDependencies", endTs, defaultDependencyLookbackDuration).Return(nil, errStorage).Times(1)

	var response structuredResponse
	err := getJSON(server.URL+"/api/dependencies?endTs=1476374248550&service=testing", &response)
	assert.Error(t, err)
}

func TestGetDependenciesEndTimeParsingFailure(t *testing.T) {
	server, _, _ := initializeTestServer()
	defer server.Close()

	var response structuredResponse
	err := getJSON(server.URL+"/api/dependencies?endTs=shazbot&service=testing", &response)
	assert.Error(t, err)
}

func TestGetDependenciesLookbackParsingFailure(t *testing.T) {
	server, _, _ := initializeTestServer()
	defer server.Close()

	var response structuredResponse
	err := getJSON(server.URL+"/api/dependencies?endTs=1476374248550&service=testing&lookback=shazbot", &response)
	assert.Error(t, err)
}
