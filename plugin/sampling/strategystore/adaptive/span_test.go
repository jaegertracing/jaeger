// Copyright (c) 2021 The Jaeger Authors.
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

package adaptive

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/model"
)

func TestGetSamplerParams(t *testing.T) {
	logger := zap.NewNop()
	tests := []struct {
		tags          model.KeyValues
		expectedType  string
		expectedParam float64
	}{
		{
			tags: model.KeyValues{
				model.String("sampler.type", "probabilistic"),
				model.String("sampler.param", "1e-05"),
			},
			expectedType:  "probabilistic",
			expectedParam: 0.00001,
		},
		{
			tags: model.KeyValues{
				model.String("sampler.type", "probabilistic"),
				model.Float64("sampler.param", 0.10404450002098709),
			},
			expectedType:  "probabilistic",
			expectedParam: 0.10404450002098709,
		},
		{
			tags: model.KeyValues{
				model.String("sampler.type", "probabilistic"),
				model.String("sampler.param", "0.10404450002098709"),
			},
			expectedType:  "probabilistic",
			expectedParam: 0.10404450002098709,
		},
		{
			tags: model.KeyValues{
				model.String("sampler.type", "probabilistic"),
				model.Int64("sampler.param", 1),
			},
			expectedType:  "probabilistic",
			expectedParam: 1.0,
		},
		{
			tags: model.KeyValues{
				model.String("sampler.type", "ratelimiting"),
				model.String("sampler.param", "1"),
			},
			expectedType:  "ratelimiting",
			expectedParam: 1,
		},
		{
			tags: model.KeyValues{
				model.Float64("sampler.type", 1.5),
			},
			expectedType:  "",
			expectedParam: 0,
		},
		{
			tags: model.KeyValues{
				model.String("sampler.type", "probabilistic"),
			},
			expectedType:  "",
			expectedParam: 0,
		},
		{
			tags:          model.KeyValues{},
			expectedType:  "",
			expectedParam: 0,
		},
		{
			tags: model.KeyValues{
				model.String("sampler.type", "lowerbound"),
				model.String("sampler.param", "1"),
			},
			expectedType:  "lowerbound",
			expectedParam: 1,
		},
		{
			tags: model.KeyValues{
				model.String("sampler.type", "lowerbound"),
				model.Int64("sampler.param", 1),
			},
			expectedType:  "lowerbound",
			expectedParam: 1,
		},
		{
			tags: model.KeyValues{
				model.String("sampler.type", "lowerbound"),
				model.Float64("sampler.param", 0.5),
			},
			expectedType:  "lowerbound",
			expectedParam: 0.5,
		},
		{
			tags: model.KeyValues{
				model.String("sampler.type", "lowerbound"),
				model.String("sampler.param", "not_a_number"),
			},
			expectedType:  "",
			expectedParam: 0,
		},
	}

	for i, test := range tests {
		tt := test
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			span := &model.Span{}
			span.Tags = tt.tags
			actualType, actualParam := GetSamplerParams(span, logger)
			assert.Equal(t, tt.expectedType, actualType)
			assert.Equal(t, tt.expectedParam, actualParam)
		})
	}
}
