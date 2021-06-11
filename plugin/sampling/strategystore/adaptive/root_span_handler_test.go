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
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/model"
)

type mockAggregator struct {
	callCount int
}

func (t *mockAggregator) RecordThroughput(service, operation, samplerType string, probability float64) {
	t.callCount++
}
func (t *mockAggregator) Start() {}
func (t *mockAggregator) Stop()  {}

func TestHandleRootSpan(t *testing.T) {
	aggregator := &mockAggregator{}
	processor := HandleRootSpan(aggregator, zap.NewNop())

	// Testing non-root span
	span := &model.Span{References: []model.SpanRef{{SpanID: model.NewSpanID(1), RefType: model.ChildOf}}}
	processor(span)
	assert.Equal(t, 0, aggregator.callCount)

	// Testing span with service name but no operation
	span.References = []model.SpanRef{}
	span.Process = &model.Process{
		ServiceName: "service",
	}
	processor(span)
	assert.Equal(t, 0, aggregator.callCount)

	// Testing span with service name and operation but no probabilistic sampling tags
	span.OperationName = "GET"
	processor(span)
	assert.Equal(t, 0, aggregator.callCount)

	// Testing span with service name, operation, and probabilistic sampling tags
	span.Tags = model.KeyValues{
		model.String("sampler.type", "probabilistic"),
		model.String("sampler.param", "0.001"),
	}
	processor(span)
	assert.Equal(t, 1, aggregator.callCount)
}
