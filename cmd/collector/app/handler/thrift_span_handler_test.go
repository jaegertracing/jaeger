// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package handler

import (
	"encoding/json"
	"errors"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/cmd/collector/app/processor"
	zipkinsanitizer "github.com/jaegertracing/jaeger/cmd/collector/app/sanitizer/zipkin"
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
			require.NoError(t, err)
			assert.True(t, res[0].Ok)
		}
	}
}

type shouldIErrorProcessor struct {
	shouldError bool
}

var errTestError = errors.New("Whoops")

func (s *shouldIErrorProcessor) ProcessSpans(spans processor.Spans) ([]bool, error) {
	if s.shouldError {
		return nil, errTestError
	}
	retMe := make([]bool, len(spans.SpansV1))
	for i := range spans.SpansV1 {
		retMe[i] = true
	}
	return retMe, nil
}

func (*shouldIErrorProcessor) Close() error {
	return nil
}

func TestZipkinSpanHandler(t *testing.T) {
	tests := []struct {
		name        string
		expectedErr error
		filename    string
	}{
		{
			name:        "good case",
			expectedErr: nil,
		},
		{
			name:        "bad case",
			expectedErr: errTestError,
		},
		{
			name:        "dual client-server span",
			expectedErr: nil,
			filename:    "testdata/zipkin_thrift_v1_merged_spans.json",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			logger := zap.NewNop()
			h := NewZipkinSpanHandler(
				logger,
				&shouldIErrorProcessor{tc.expectedErr != nil},
				zipkinsanitizer.NewChainedSanitizer(zipkinsanitizer.NewStandardSanitizers()...),
			)
			var spans []*zipkincore.Span
			if tc.filename != "" {
				data, err := os.ReadFile(tc.filename)
				require.NoError(t, err)
				require.NoError(t, json.Unmarshal(data, &spans))
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
				assert.Len(t, res, len(spans))
				require.NoError(t, err)
				for i := range res {
					assert.True(t, res[i].Ok)
				}
			}
		})
	}
}
