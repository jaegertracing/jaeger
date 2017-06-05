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
		esSpan := FromDomainEmbedProcess(&span)

		var expectedSpan jModel.Span
		require.NoError(t, json.Unmarshal(outStr, &expectedSpan))

		CompareJSONSpans(t, &expectedSpan, esSpan)
	}
}

func testReadFixtures(t *testing.T, i int, es bool) ([]byte, []byte) {
	var in string
	if es {
		in = fmt.Sprintf("fixtures/domain_es_%02d.json", i)
	} else {
		in = fmt.Sprintf("fixtures/domain_%02d.json", i)
	}
	inStr, err := ioutil.ReadFile(in)
	require.NoError(t, err)
	var out string
	if es {
		out = fmt.Sprintf("fixtures/es_%02d.json", i)
	} else {
		out = fmt.Sprintf("fixtures/ui_%02d.json", i)
	}
	outStr, err := ioutil.ReadFile(out)
	require.NoError(t, err)
	return inStr, outStr
}

func testOutput(t *testing.T, i int, outStr []byte, object interface{}, es bool) {
	buf := &bytes.Buffer{}
	enc := json.NewEncoder(buf)
	enc.SetIndent("", "  ")

	var outFile string
	if es {
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
