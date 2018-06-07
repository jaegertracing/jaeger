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

package json

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"testing"
	"time"

	"github.com/gogo/protobuf/jsonpb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger/model"
	jModel "github.com/jaegertracing/jaeger/model/json"
)

const NumberOfFixtures = 1

func TestMarshalJSON(t *testing.T) {
	span1 := &model.Span{
		TraceID:       model.TraceID{Low: 1},
		SpanID:        model.SpanID(2),
		OperationName: "span",
		StartTime:     time.Now(),
		Duration:      time.Microsecond,
	}
	trace1 := &model.Trace{
		Spans: []*model.Span{
			span1,
		},
		ProcessMap: []model.Trace_ProcessMapping{
			{
				ProcessID: "p1",
				Process: model.Process{
					ServiceName: "abc",
				},
			},
		},
	}
	m := &jsonpb.Marshaler{}
	out := &bytes.Buffer{}

	require.NoError(t, m.Marshal(out, trace1))

	var trace2 model.Trace
	bb := bytes.NewReader(out.Bytes())
	require.NoError(t, jsonpb.Unmarshal(bb, &trace2))
	trace1.NormalizeTimestamps()
	trace2.NormalizeTimestamps()
	assert.Equal(t, trace1, &trace2)
}

func TestFromDomain(t *testing.T) {
	for i := 1; i <= NumberOfFixtures; i++ {
		domainStr, jsonStr := loadFixturesUI(t, i)

		var trace model.Trace
		require.NoError(t, jsonpb.Unmarshal(bytes.NewReader(domainStr), &trace))
		uiTrace := FromDomain(&trace)

		testJSONEncoding(t, i, jsonStr, uiTrace, false)
	}
}

func TestFromDomainEmbedProcess(t *testing.T) {
	for i := 1; i <= NumberOfFixtures; i++ {
		domainStr, jsonStr := loadFixturesES(t, i)

		var span model.Span
		require.NoError(t, jsonpb.Unmarshal(bytes.NewReader(domainStr), &span))
		embeddedSpan := FromDomainEmbedProcess(&span)

		var expectedSpan jModel.Span
		require.NoError(t, json.Unmarshal(jsonStr, &expectedSpan))

		testJSONEncoding(t, i, jsonStr, embeddedSpan, true)

		CompareJSONSpans(t, &expectedSpan, embeddedSpan)
	}
}

func loadFixturesUI(t *testing.T, i int) ([]byte, []byte) {
	return loadFixtures(t, i, false)
}

func loadFixturesES(t *testing.T, i int) ([]byte, []byte) {
	return loadFixtures(t, i, true)
}

// Loads and returns domain model and JSON model fixtures with given number i.
func loadFixtures(t *testing.T, i int, processEmbedded bool) ([]byte, []byte) {
	var in string
	if processEmbedded {
		in = fmt.Sprintf("fixtures/domain_es_%02d.json", i)
	} else {
		in = fmt.Sprintf("fixtures/domain_%02d.json", i)
	}
	inStr, err := ioutil.ReadFile(in)
	require.NoError(t, err)
	var out string
	if processEmbedded {
		out = fmt.Sprintf("fixtures/es_%02d.json", i)
	} else {
		out = fmt.Sprintf("fixtures/ui_%02d.json", i)
	}
	outStr, err := ioutil.ReadFile(out)
	require.NoError(t, err)
	return inStr, outStr
}

func testJSONEncoding(t *testing.T, i int, expectedStr []byte, object interface{}, processEmbedded bool) {
	buf := &bytes.Buffer{}
	enc := json.NewEncoder(buf)
	enc.SetIndent("", "  ")

	var outFile string
	if processEmbedded {
		outFile = fmt.Sprintf("fixtures/es_%02d", i)
	} else {
		outFile = fmt.Sprintf("fixtures/ui_%02d", i)
	}
	require.NoError(t, enc.Encode(object))

	if !assert.Equal(t, string(expectedStr), string(buf.Bytes())) {
		err := ioutil.WriteFile(outFile+"-actual.json", buf.Bytes(), 0644)
		assert.NoError(t, err)
	}
}

func TestDependenciesFromDomain(t *testing.T) {
	someParent := "someParent"
	someChild := "someChild"
	someCallCount := uint64(123)
	anotherParent := "anotherParent"
	anotherChild := "anotherChild"
	anotherCallCount := uint64(456)
	expected := []jModel.DependencyLink{
		{
			Parent:    someParent,
			Child:     someChild,
			CallCount: someCallCount,
		},
		{
			Parent:    anotherParent,
			Child:     anotherChild,
			CallCount: anotherCallCount,
		},
	}
	input := []model.DependencyLink{
		{
			Parent:    someParent,
			Child:     someChild,
			CallCount: someCallCount,
		},
		{
			Parent:    anotherParent,
			Child:     anotherChild,
			CallCount: anotherCallCount,
		},
	}
	actual := DependenciesFromDomain(input)
	assert.EqualValues(t, expected, actual)
}
