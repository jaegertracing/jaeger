// Copyright (c) 2019 The Jaeger Authors.
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

package jaeger

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaegertracing/jaeger/proto-gen/api_v2"
	"github.com/jaegertracing/jaeger/thrift-gen/baggage"
)

func TestConvertBaggageRestrictionsResponseFromDomain(t *testing.T) {
	tests := []struct {
		expected []*baggage.BaggageRestriction
		in       *api_v2.BaggageRestrictionResponse
	}{
		{
			expected: []*baggage.BaggageRestriction{
				{
					BaggageKey:     "key",
					MaxValueLength: 5,
				},
			},
			in: &api_v2.BaggageRestrictionResponse{
				BaggageRestrictions: []*api_v2.BaggageRestriction{
					{
						BaggageKey:     "key",
						MaxValueLength: 5,
					},
				},
			},
		},
		{},
	}
	for _, test := range tests {
		actual, err := ConvertBaggageRestrictionsResponseFromDomain(test.in)
		assert.NoError(t, err)
		assert.Equal(t, test.expected, actual)
	}
}
