// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package sanitizer

import (
	"testing"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/stretchr/testify/assert"
)

var testCache = map[string]string{
	"rt-supply": "rt-supply",
	"supply":    "rt-supply",
}

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

func (*fixedMappingCache) Put(string /* key */, string /* value */) error {
	return nil
}

func (*fixedMappingCache) Initialize() error {
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
