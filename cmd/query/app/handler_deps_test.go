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
