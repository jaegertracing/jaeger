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

package sanitizer

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/uber/jaeger/model"
)

var (
	testCache = map[string]string{
		"rt-supply": "rt-supply",
		"supply":    "rt-supply",
	}

	emptyCache = map[string]string{}
)

type fixedMappingCache struct{
	Cache map[string]string
}

func (d *fixedMappingCache) Get(key string) string {
	k, ok := d.Cache[key]
	if !ok {
		return ""
	}
	return k
}

func (d *fixedMappingCache) Put(key string, value string) error {
	return nil
}

func (d *fixedMappingCache) Initialize() error {
	return nil
}

func (d *fixedMappingCache) IsEmpty() bool {
	return len(d.Cache) == 0
}

func getImpl(c map[string]string) SanitizeSpan {
	return NewChainedSanitizer(NewServiceNameSanitizer(&fixedMappingCache{
		Cache: c,
	}))
}

func TestSanitize(t *testing.T) {
	i := getImpl(testCache)

	tests := []struct {
		incomingName string
		expectedName string
	}{
		{
			"supply",
			"rt-supply",
		},
		{
			"rt-supply",
			"rt-supply",
		},
		{
			"bad_name",
			"bad_name",
		},
	}
	for _, test := range tests {
		rawSpan := &model.Span{
			Process: &model.Process{
				ServiceName: test.incomingName,
			},
		}
		span := i(rawSpan)
		assert.Equal(t, test.expectedName, span.Process.ServiceName)
	}
}

func TestSanitizeEmptyCache(t *testing.T) {
	i := getImpl(emptyCache)

	incomingName := "supply"
	expectedName := incomingName
	
	rawSpan := &model.Span{
		Process: &model.Process{
			ServiceName: incomingName,
		},
	}

	span := i(rawSpan)
	assert.Equal(t, expectedName, span.Process.ServiceName)
}
