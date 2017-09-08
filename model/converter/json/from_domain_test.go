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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/uber/jaeger/model"
	jModel "github.com/uber/jaeger/model/json"
)

const NumberOfFixtures = 1

func TestFromDomain(t *testing.T) {
	for i := 1; i <= NumberOfFixtures; i++ {
		inStr, outStr := testReadFixtures(t, i, false)

		var trace model.Trace
		require.NoError(t, json.Unmarshal(inStr, &trace))
		uiTrace := FromDomain(&trace)

		testOutput(t, i, outStr, uiTrace, false)
	}
	// this is just to confirm the uint64 representation of float64(72.5) used as a "temperature" tag
	assert.Equal(t, int64(4634802150889750528), model.Float64("x", 72.5).VNum)
}

func TestFromDomainEmbedProcess(t *testing.T) {
	for i := 1; i <= NumberOfFixtures; i++ {
		inStr, outStr := testReadFixtures(t, i, true)

		var span model.Span
		require.NoError(t, json.Unmarshal(inStr, &span))
		embeddedSpan := FromDomainEmbedProcess(&span)

		var expectedSpan jModel.Span
		require.NoError(t, json.Unmarshal(outStr, &expectedSpan))

		CompareJSONSpans(t, &expectedSpan, embeddedSpan)
	}
}

func testReadFixtures(t *testing.T, i int, processEmbedded bool) ([]byte, []byte) {
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

func testOutput(t *testing.T, i int, outStr []byte, object interface{}, processEmbedded bool) {
	buf := &bytes.Buffer{}
	enc := json.NewEncoder(buf)
	enc.SetIndent("", "  ")

	var outFile string
	if processEmbedded {
		outFile = fmt.Sprintf("fixtures/es_%02d", i)
		require.NoError(t, enc.Encode(object.(*jModel.Span)))
	} else {
		outFile = fmt.Sprintf("fixtures/ui_%02d", i)
		require.NoError(t, enc.Encode(object.(*jModel.Trace)))
	}

	if !assert.Equal(t, string(outStr), string(buf.Bytes())) {
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
