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

package json_test

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

	. "github.com/uber/jaeger/model/converter/json"
)

const NumberOfFixtures = 1

func TestFromDomain(t *testing.T) {
	for i := 1; i <= NumberOfFixtures; i++ {
		in := fmt.Sprintf("fixtures/domain_%02d.json", i)
		inStr, err := ioutil.ReadFile(in)
		require.NoError(t, err)
		var trace model.Trace
		require.NoError(t, json.Unmarshal(inStr, &trace))

		out := fmt.Sprintf("fixtures/ui_%02d.json", i)
		outStr, err := ioutil.ReadFile(out)
		require.NoError(t, err)

		uiTrace := FromDomain(&trace)

		buf := &bytes.Buffer{}
		enc := json.NewEncoder(buf)
		enc.SetIndent("", "  ")
		require.NoError(t, enc.Encode(uiTrace))
		actual := string(buf.Bytes())

		if !assert.Equal(t, string(outStr), actual) {
			err := ioutil.WriteFile(out+"-actual", buf.Bytes(), 0644)
			assert.NoError(t, err)
		}
	}
	// this is just to confirm the uint64 representation of float64(72.5) used as a "temperature" tag
	assert.Equal(t, int64(4634802150889750528), model.Float64("x", 72.5).VNum)
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

func TestFromDomainES(t *testing.T) {
	for i := 1; i <= NumberOfFixtures; i++ {
		in := fmt.Sprintf("fixtures/domain_es_%02d.json", i)
		inStr, err := ioutil.ReadFile(in)
		require.NoError(t, err)
		var span model.Span
		require.NoError(t, json.Unmarshal(inStr, &span))

		out := fmt.Sprintf("fixtures/es_%02d.json", i)
		outStr, err := ioutil.ReadFile(out)
		require.NoError(t, err)

		uiTrace := FromDomainES(&span)

		buf := &bytes.Buffer{}
		enc := json.NewEncoder(buf)
		enc.SetIndent("", "  ")
		require.NoError(t, enc.Encode(uiTrace))
		actual := string(buf.Bytes())

		if !assert.Equal(t, string(outStr), actual) {
			err := ioutil.WriteFile(out+"-actual", buf.Bytes(), 0644)
			assert.NoError(t, err)
		}
	}
	// this is just to confirm the uint64 representation of float64(72.5) used as a "temperature" tag
	assert.Equal(t, int64(4634802150889750528), model.Float64("x", 72.5).VNum)
}