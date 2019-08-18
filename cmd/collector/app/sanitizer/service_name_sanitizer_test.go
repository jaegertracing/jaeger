// Copyright (c) 2019 The Jaeger Authors.
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

package sanitizer

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaegertracing/jaeger/model"
)

var (
	testCache = map[string]string{
		"rt-supply": "rt-supply",
		"supply":    "rt-supply",
	}
)

type fixedMappingCache struct {
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
	i := getImpl(make(map[string]string))

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
