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
	"github.com/jaegertracing/jaeger/proto-gen/api_v2"
	"github.com/jaegertracing/jaeger/thrift-gen/baggage"
)

// ConvertBaggageRestrictionsResponseFromDomain converts proto baggageRestrictions response to its thrift representation
func ConvertBaggageRestrictionsResponseFromDomain(r *api_v2.BaggageRestrictionResponse) ([]*baggage.BaggageRestriction, error) {
	if r == nil || r.BaggageRestrictions == nil {
		return nil, nil
	}
	res := make([]*baggage.BaggageRestriction, len(r.BaggageRestrictions))
	for i, bg := range r.BaggageRestrictions {
		res[i] = &baggage.BaggageRestriction{
			BaggageKey:     bg.BaggageKey,
			MaxValueLength: bg.MaxValueLength,
		}
	}
	return res, nil
}
