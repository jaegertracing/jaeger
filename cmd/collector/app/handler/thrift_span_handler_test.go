// Copyright (c) 2019 The Jaeger Authors.
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

package handler

import (
	"errors"
	"io/ioutil"
	"testing"

	"github.com/crossdock/crossdock-go/require"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/collector/app/processor"
	zipkinsanitizer "github.com/jaegertracing/jaeger/cmd/collector/app/sanitizer/zipkin"
	"github.com/jaegertracing/jaeger/cmd/collector/app/zipkin/zipkindeser"
	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/thrift-gen/jaeger"
	"github.com/jaegertracing/jaeger/thrift-gen/zipkincore"
)

func TestJaegerSpanHandler(t *testing.T) {
	testChunks := []struct {
		expectedErr error
	}{
		{
			expectedErr: nil,
		},
		{
			expectedErr: errTestError,
		},
	}
	for _, tc := range testChunks {
		logger := zap.NewNop()
		h := NewJaegerSpanHandler(logger, &shouldIErrorProcessor{tc.expectedErr != nil})
		res, err := h.SubmitBatches([]*jaeger.Batch{
			{
				Process: &jaeger.Process{ServiceName: "someServiceName"},
				Spans:   []*jaeger.Span{{SpanId: 21345}},
			},
		}, SubmitBatchOptions{})
		if tc.expectedErr != nil {
			assert.Nil(t, res)
			assert.Equal(t, tc.expectedErr, err)
		} else {
			assert.Len(t, res, 1)
			assert.NoError(t, err)
			assert.True(t, res[0].Ok)
		}
	}
}

type shouldIErrorProcessor struct {
	shouldError bool
}

var errTestError = errors.New("Whoops")

func (s *shouldIErrorProcessor) ProcessSpans(mSpans []*model.Span, _ processor.SpansOptions) ([]bool, error) {
	if s.shouldError {
		return nil, errTestError
	}
	retMe := make([]bool, len(mSpans))
	for i := range mSpans {
		retMe[i] = true
	}
	return retMe, nil
}

func (s *shouldIErrorProcessor) Close() error {
	return nil
}

func TestZipkinSpanHandler(t *testing.T) {
	tests := []struct {
		expectedErr   error
		expectedCount int
		filename      string
		inputCount    int // how many spans are expected in the input file
	}{
		{
			expectedErr:   nil,
			expectedCount: 1,
		},
		{
			expectedErr: errTestError,
		},
		{
			expectedErr:   nil,
			filename:      "testdata/zipkin_v1_merged_spans.json",
			inputCount:    2,
			expectedCount: 3,
		},
	}
	for _, tc := range tests {
		logger := zap.NewNop()
		h := NewZipkinSpanHandler(
			logger,
			&shouldIErrorProcessor{tc.expectedErr != nil},
			zipkinsanitizer.NewParentIDSanitizer(),
		)
		var spans []*zipkincore.Span
		if tc.filename != "" {
			data, err := ioutil.ReadFile(tc.filename)
			require.NoError(t, err)
			spans, err = zipkindeser.DeserializeJSON(data)
			require.NoError(t, err)
			require.EqualValues(t, tc.inputCount, len(spans))
		} else {
			spans = []*zipkincore.Span{
				{
					ID: 12345,
				},
			}
		}
		res, err := h.SubmitZipkinBatch(spans, SubmitBatchOptions{})
		if tc.expectedErr != nil {
			assert.Nil(t, res)
			assert.Equal(t, tc.expectedErr, err)
		} else {
			assert.Len(t, res, tc.expectedCount)
			assert.NoError(t, err)
			assert.True(t, res[0].Ok)
		}
	}
}

func TestZipkinSpanHandler_MergedSpans(t *testing.T) {

}
