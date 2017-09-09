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

package model_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/uber/jaeger/model"
)

func TestTraceFindSpanByID(t *testing.T) {
	trace := &model.Trace{
		Spans: []*model.Span{
			{SpanID: model.SpanID(1), OperationName: "x"},
			{SpanID: model.SpanID(2), OperationName: "y"},
			{SpanID: model.SpanID(1), OperationName: "z"}, // same span ID
		},
	}
	s1 := trace.FindSpanByID(model.SpanID(1))
	assert.NotNil(t, s1)
	assert.Equal(t, "x", s1.OperationName)
	s2 := trace.FindSpanByID(model.SpanID(2))
	assert.NotNil(t, s2)
	assert.Equal(t, "y", s2.OperationName)
	s3 := trace.FindSpanByID(model.SpanID(3))
	assert.Nil(t, s3)
}

func TestTraceNormalizeTimestamps(t *testing.T) {
	s1 := "2017-01-26T16:46:31.639875-05:00"
	s2 := "2017-01-26T21:46:31.639875-04:00"
	var tt1, tt2 time.Time
	assert.NoError(t, tt1.UnmarshalText([]byte(s1)))
	assert.NoError(t, tt2.UnmarshalText([]byte(s2)))

	trace := &model.Trace{
		Spans: []*model.Span{
			{
				StartTime: tt1,
				Logs: []model.Log{
					{
						Timestamp: tt2,
					},
				},
			},
		},
	}
	trace.NormalizeTimestamps()
	span := trace.Spans[0]
	assert.Equal(t, span.StartTime, tt1.UTC())
	assert.Equal(t, span.Logs[0].Timestamp, tt2.UTC())
}
