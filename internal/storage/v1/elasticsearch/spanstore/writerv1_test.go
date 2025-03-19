// Copyright (c) 2025 The Jaeger Authors.
// SPDX-License-Identifier: Apache-2.0

package spanstore

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/internal/metricstest"
	"github.com/jaegertracing/jaeger/internal/storage/v1/elasticsearch/spanstore/internal/dbmodel"
	writerMocks "github.com/jaegertracing/jaeger/internal/storage/v1/elasticsearch/spanstore/mocks"
	"github.com/jaegertracing/jaeger/internal/testutils"
	"github.com/jaegertracing/jaeger/pkg/es"
	"github.com/jaegertracing/jaeger/pkg/es/mocks"
)

func TestNewSpanTags(t *testing.T) {
	client := &mocks.Client{}
	clientFn := func() es.Client { return client }
	logger, _ := testutils.NewLogger()
	metricsFactory := metricstest.NewFactory(0)
	testCases := []struct {
		writer   *SpanWriterV1
		expected dbmodel.Span
		name     string
	}{
		{
			writer: NewSpanWriterV1(SpanWriterParams{
				Client: clientFn, Logger: logger, MetricsFactory: metricsFactory,
				AllTagsAsFields: true,
			}),
			expected: dbmodel.Span{
				Tag: map[string]any{"foo": "bar"}, Tags: []dbmodel.KeyValue{},
				Process: dbmodel.Process{Tag: map[string]any{"bar": "baz"}, Tags: []dbmodel.KeyValue{}},
			},
			name: "allTagsAsFields",
		},
		{
			writer: NewSpanWriterV1(SpanWriterParams{
				Client: clientFn, Logger: logger, MetricsFactory: metricsFactory,
				TagKeysAsFields: []string{"foo", "bar", "rere"},
			}),
			expected: dbmodel.Span{
				Tag: map[string]any{"foo": "bar"}, Tags: []dbmodel.KeyValue{},
				Process: dbmodel.Process{Tag: map[string]any{"bar": "baz"}, Tags: []dbmodel.KeyValue{}},
			},
			name: "definedTagNames",
		},
		{
			writer: NewSpanWriterV1(SpanWriterParams{Client: clientFn, Logger: logger, MetricsFactory: metricsFactory}),
			expected: dbmodel.Span{
				Tags: []dbmodel.KeyValue{{
					Key:   "foo",
					Type:  dbmodel.StringType,
					Value: "bar",
				}},
				Process: dbmodel.Process{Tags: []dbmodel.KeyValue{{
					Key:   "bar",
					Type:  dbmodel.StringType,
					Value: "baz",
				}}},
			},
			name: "noAllTagsAsFields",
		},
	}

	s := &model.Span{
		Tags:    []model.KeyValue{{Key: "foo", VStr: "bar"}},
		Process: &model.Process{Tags: []model.KeyValue{{Key: "bar", VStr: "baz"}}},
	}
	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			mSpan := test.writer.spanConverter.FromDomainEmbedProcess(s)
			assert.Equal(t, test.expected.Tag, mSpan.Tag)
			assert.Equal(t, test.expected.Tags, mSpan.Tags)
			assert.Equal(t, test.expected.Process.Tag, mSpan.Process.Tag)
			assert.Equal(t, test.expected.Process.Tags, mSpan.Process.Tags)
		})
	}
}

func TestSpanWriterV1_WriteSpan(t *testing.T) {
	coreWriter := &writerMocks.CoreSpanWriter{}
	s := &model.Span{
		Tags:    []model.KeyValue{{Key: "foo", VStr: "bar"}},
		Process: &model.Process{Tags: []model.KeyValue{{Key: "bar", VStr: "baz"}}},
	}
	converter := dbmodel.NewFromDomain(true, []string{}, "-")
	writerV1 := &SpanWriterV1{spanWriter: coreWriter, spanConverter: converter}
	coreWriter.On("WriteSpan", s.StartTime, converter.FromDomainEmbedProcess(s))
	err := writerV1.WriteSpan(context.Background(), s)
	require.NoError(t, err)
}
