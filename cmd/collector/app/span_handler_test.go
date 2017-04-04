// Copyright (c) 2017 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package app

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/uber/tchannel-go/thrift"
	"go.uber.org/zap"

	"github.com/uber/jaeger/cmd/collector/app/sanitizer/zipkin"
	"github.com/uber/jaeger/model"
	"github.com/uber/jaeger/thrift-gen/jaeger"
	"github.com/uber/jaeger/thrift-gen/zipkincore"
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
		ctx, cancel := thrift.NewContext(time.Minute)
		defer cancel()
		res, err := h.SubmitBatches(ctx, []*jaeger.Batch{
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

func (s *shouldIErrorProcessor) ProcessSpans(mSpans []*model.Span, format string) ([]bool, error) {
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
		h := NewZipkinSpanHandler(logger, &shouldIErrorProcessor{tc.expectedErr != nil}, zipkin.NewParentIDSanitizer(logger))
		ctx, cancel := thrift.NewContext(time.Minute)
		defer cancel()
		res, err := h.SubmitZipkinBatch(ctx, []*zipkincore.Span{
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
