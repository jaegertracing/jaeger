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

package app

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/collector/app/sanitizer/zipkin"
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

		res, err = h.SubmitHTTPBatches(ctx, []*jaeger.Batch{
			{
				Process: &jaeger.Process{ServiceName: "someServiceName"},
				Spans:   []*jaeger.Span{{SpanId: 21345}},
			},
		})
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

func (s *shouldIErrorProcessor) ProcessSpans(mSpans []*model.Span, _ ProcessSpansOptions) ([]bool, error) {
	if s.shouldError {
		return nil, errTestError
	}
	retMe := make([]bool, len(mSpans))
	for i := range mSpans {
		retMe[i] = true
	}
	return retMe, nil
}

func TestZipkinSpanHandler(t *testing.T) {
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
		h := NewZipkinSpanHandler(logger, &shouldIErrorProcessor{tc.expectedErr != nil}, zipkin.NewParentIDSanitizer())
		res, err := h.SubmitZipkinBatch([]*zipkincore.Span{
			{
				ID: 12345,
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

		res, err = h.SubmitHTTPZipkinBatch(ctx, []*zipkincore.Span{
			{
				ID: 12345,
			},
		})
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
