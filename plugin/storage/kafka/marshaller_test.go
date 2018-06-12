// Copyright (c) 2018 The Jaeger Authors.
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

package kafka

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaegertracing/jaeger/model"
)

func TestThriftMarshaller(t *testing.T) {
	tags := model.KeyValues{
		model.String("str", "Stringer Bell"),
		model.Float64("floating", 1.1),
		model.Int64("int", 112),
		model.Bool("bool", true),
		model.Binary("binary", []byte("Omar")),
	}
	span := &model.Span{
		TraceID:       model.TraceID{High: 22222, Low: 44444},
		SpanID:        model.SpanID(3333),
		OperationName: "operator",
		References: []model.SpanRef{
			{
				TraceID: model.TraceID{High: 22222, Low: 44444},
				SpanID:  model.SpanID(11111),
				RefType: model.ChildOf,
			},
		},
		Flags:     model.Flags(1),
		StartTime: model.EpochMicrosecondsAsTime(55555),
		Duration:  model.MicrosecondsAsDuration(50000),
		Tags:      tags,
		Logs: []model.Log{
			{
				Timestamp: model.EpochMicrosecondsAsTime(12345),
				Fields:    tags,
			},
		},
		Process: &model.Process{
			ServiceName: "someServiceName",
			Tags:        tags,
		},
	}

	marshaller := newThriftMarshaller()

	bytes, err := marshaller.Marshal(span)

	assert.NoError(t, err)
	assert.NotNil(t, bytes)
}
