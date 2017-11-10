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

package adjuster_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/model/adjuster"
)

func TestSequences(t *testing.T) {
	// mock adjuster that increments span ID
	adj := adjuster.Func(func(trace *model.Trace) (*model.Trace, error) {
		trace.Spans[0].SpanID++
		return trace, nil
	})

	adjErr := errors.New("mock adjuster error")
	failingAdj := adjuster.Func(func(trace *model.Trace) (*model.Trace, error) {
		return trace, adjErr
	})

	testCases := []struct {
		adjuster   adjuster.Adjuster
		err        string
		lastSpanID int
	}{
		{
			adjuster:   adjuster.Sequence(adj, failingAdj, adj, failingAdj),
			err:        fmt.Sprintf("[%s, %s]", adjErr, adjErr),
			lastSpanID: 2,
		},
		{
			adjuster:   adjuster.FailFastSequence(adj, failingAdj, adj, failingAdj),
			err:        adjErr.Error(),
			lastSpanID: 1,
		},
	}

	for _, testCase := range testCases {
		span := &model.Span{}
		trace := model.Trace{Spans: []*model.Span{span}}

		adjTrace, err := testCase.adjuster.Adjust(&trace)

		assert.True(t, span == adjTrace.Spans[0], "same trace & span returned")
		assert.EqualValues(t, testCase.lastSpanID, span.SpanID, "expect span ID to be incremented")
		assert.EqualError(t, err, testCase.err)
	}
}
