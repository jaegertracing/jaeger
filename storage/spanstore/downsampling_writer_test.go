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

package spanstore

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaegertracing/jaeger/model"
)

type noopWriteSpanStore struct{}

func (n *noopWriteSpanStore) WriteSpan(span *model.Span) error {
	return nil
}
func TestDownSamplingWriter_WriteSpan(t *testing.T) {
	trace := model.TraceID{
		Low:  uint64(0),
		High: uint64(1),
	}
	span := &model.Span{
		TraceID: trace,
	}
	downSamplingOptions := DownSamplingOptions{
		Ratio:    1,
		HashSalt: "jaeger-test",
	}
	c := NewDownSamplingWriter(&noopWriteSpanStore{}, downSamplingOptions)
	assert.NoError(t, c.WriteSpan(span))

	downSamplingOptions.Ratio = 0
	c = NewDownSamplingWriter(&noopWriteSpanStore{}, downSamplingOptions)
	assert.NoError(t, c.WriteSpan(span))

	downSamplingOptions.Ratio = 0.8
	c = NewDownSamplingWriter(&noopWriteSpanStore{}, downSamplingOptions)
	assert.NoError(t, c.WriteSpan(span))
}

func TestDownSamplingWriter_hashBytes(t *testing.T) {
	downSamplingOptions := DownSamplingOptions{
		Ratio:          1,
		HashSalt:       "",
		MetricsFactory: nil,
	}
	c := NewDownSamplingWriter(&noopWriteSpanStore{}, downSamplingOptions)
	h := c.pool.Get().(*hasher)
	assert.Equal(t, h.hashBytes(nil), h.hashBytes(nil))
	c.pool.Put(h)
	trace := model.TraceID{
		Low:  uint64(0),
		High: uint64(1),
	}
	span := &model.Span{
		TraceID: trace,
	}
	traceIDBytes := make([]byte, 16)
	span.TraceID.MarshalTo(traceIDBytes)
	// Same traceID should always be hashed to same uint64 in DownSamplingWriter
	assert.Equal(t, h.hashBytes(traceIDBytes), h.hashBytes(traceIDBytes))
}
