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

package spanstore_test

import (
	"hash/fnv"
	"math"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/storage/spanstore"
	. "github.com/jaegertracing/jaeger/storage/spanstore"
)

func TestDownSamplingWriter_WriteSpan(t *testing.T) {
	trace := model.TraceID{
		Low:  uint64(0),
		High: uint64(1),
	}
	span := &model.Span{
		TraceID: trace,
	}
	downSamplingOptions := spanstore.DownSamplingOptions{
		Ratio:    1,
		HashSalt: "jaeger-test",
	}
	c := NewDownSamplingWriter(&noopWriteSpanStore{}, downSamplingOptions)
	assert.NoError(t, c.WriteSpan(span))
}

func TestDownSamplingWriter_HashBytes(t *testing.T) {
	span := &model.Span{
		TraceID: model.TraceID{
			Low:  uint64(0),
			High: uint64(1),
		},
	}

	h := fnv.New64a()
	hash1 := HashBytes(h, []byte(span.TraceID.String()))
	hash2 := HashBytes(h, []byte(span.TraceID.String()))
	assert.Equal(t, hash1, hash2)
}
func TestDownSamplingWriter_RandomHash(t *testing.T) {
	h := fnv.New64a()
	ratioThreshold := uint64(math.MaxUint64 / 2)
	countSmallerThanRatio := 0
	for i := 0; i < 100000; i++ {
		low := rand.Uint64()
		high := rand.Uint64()
		span := &model.Span{
			TraceID: model.TraceID{
				Low:  low,
				High: high,
			},
		}
		hash := HashBytes(h, []byte(span.TraceID.String()))
		if hash < ratioThreshold {
			countSmallerThanRatio++
		}
	}
	assert.True(t, math.Abs(float64(countSmallerThanRatio)/100000-0.5) < 0.05)
}
