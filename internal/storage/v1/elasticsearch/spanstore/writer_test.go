// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package spanstore

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/internal/metricstest"
	es "github.com/jaegertracing/jaeger/internal/storage/elasticsearch"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/mocks"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/spanstore"
	"github.com/jaegertracing/jaeger/internal/storage/v1/elasticsearch/spanstore/internal/dbmodel"
	"github.com/jaegertracing/jaeger/internal/testutils"
)

type spanWriterTest struct {
	client    *mocks.Client
	logger    *zap.Logger
	logBuffer *testutils.Buffer
	writer    *SpanWriterV1
}

func withSpanWriter(fn func(w *spanWriterTest)) {
	client := &mocks.Client{}
	logger, logBuffer := testutils.NewLogger()
	metricsFactory := metricstest.NewFactory(0)
	w := &spanWriterTest{
		client:    client,
		logger:    logger,
		logBuffer: logBuffer,
		writer: NewSpanWriterV1(SpanWriterParams{
			Client: func() es.Client { return client },
			Logger: logger, MetricsFactory: metricsFactory,
			SpanIndex:    config.IndexOptions{DateLayout: "2006-01-02"},
			ServiceIndex: config.IndexOptions{DateLayout: "2006-01-02"},
		}),
	}
	fn(w)
}

var _ spanstore.Writer = &SpanWriterV1{} // check API conformance

func TestSpanWriterIndices(t *testing.T) {
	client := &mocks.Client{}
	clientFn := func() es.Client { return client }
	logger, _ := testutils.NewLogger()
	metricsFactory := metricstest.NewFactory(0)
	date := time.Now()
	spanDataLayout := "2006-01-02-15"
	serviceDataLayout := "2006-01-02"
	spanDataLayoutFormat := date.UTC().Format(spanDataLayout)
	serviceDataLayoutFormat := date.UTC().Format(serviceDataLayout)

	spanIndexOpts := config.IndexOptions{DateLayout: spanDataLayout}
	serviceIndexOpts := config.IndexOptions{DateLayout: serviceDataLayout}

	testCases := []struct {
		indices []string
		params  SpanWriterParams
	}{
		{
			params: SpanWriterParams{
				Client: clientFn, Logger: logger, MetricsFactory: metricsFactory,
				SpanIndex: spanIndexOpts, ServiceIndex: serviceIndexOpts,
			},
			indices: []string{spanIndexBaseName + spanDataLayoutFormat, serviceIndexBaseName + serviceDataLayoutFormat},
		},
		{
			params: SpanWriterParams{
				Client: clientFn, Logger: logger, MetricsFactory: metricsFactory,
				SpanIndex: spanIndexOpts, ServiceIndex: serviceIndexOpts, UseReadWriteAliases: true,
			},
			indices: []string{spanIndexBaseName + "write", serviceIndexBaseName + "write"},
		},
		{
			params: SpanWriterParams{
				Client: clientFn, Logger: logger, MetricsFactory: metricsFactory,
				SpanIndex: spanIndexOpts, ServiceIndex: serviceIndexOpts,
				WriteAliasSuffix: "archive", // ignored because UseReadWriteAliases is false
			},
			indices: []string{spanIndexBaseName + spanDataLayoutFormat, serviceIndexBaseName + serviceDataLayoutFormat},
		},
		{
			params: SpanWriterParams{
				Client: clientFn, Logger: logger, MetricsFactory: metricsFactory,
				SpanIndex: spanIndexOpts, ServiceIndex: serviceIndexOpts, IndexPrefix: "foo:",
			},
			indices: []string{"foo:" + config.IndexPrefixSeparator + spanIndexBaseName + spanDataLayoutFormat, "foo:" + config.IndexPrefixSeparator + serviceIndexBaseName + serviceDataLayoutFormat},
		},
		{
			params: SpanWriterParams{
				Client: clientFn, Logger: logger, MetricsFactory: metricsFactory,
				SpanIndex: spanIndexOpts, ServiceIndex: serviceIndexOpts, IndexPrefix: "foo:", UseReadWriteAliases: true,
			},
			indices: []string{"foo:-" + spanIndexBaseName + "write", "foo:-" + serviceIndexBaseName + "write"},
		},
		{
			params: SpanWriterParams{
				Client: clientFn, Logger: logger, MetricsFactory: metricsFactory,
				SpanIndex: spanIndexOpts, ServiceIndex: serviceIndexOpts, WriteAliasSuffix: "archive", UseReadWriteAliases: true,
			},
			indices: []string{spanIndexBaseName + "archive", serviceIndexBaseName + "archive"},
		},
		{
			params: SpanWriterParams{
				Client: clientFn, Logger: logger, MetricsFactory: metricsFactory,
				SpanIndex: spanIndexOpts, ServiceIndex: serviceIndexOpts, IndexPrefix: "foo:", WriteAliasSuffix: "archive", UseReadWriteAliases: true,
			},
			indices: []string{"foo:" + config.IndexPrefixSeparator + spanIndexBaseName + "archive", "foo:" + config.IndexPrefixSeparator + serviceIndexBaseName + "archive"},
		},
	}
	for _, testCase := range testCases {
		w := NewSpanWriterV1(testCase.params)
		spanIndexName, serviceIndexName := w.spanWriter.spanServiceIndex(date)
		assert.Equal(t, []string{spanIndexName, serviceIndexName}, testCase.indices)
	}
}

func TestClientClose(t *testing.T) {
	withSpanWriter(func(w *spanWriterTest) {
		w.client.On("Close").Return(nil)
		w.writer.Close()
		w.client.AssertNumberOfCalls(t, "Close", 1)
	})
}

// This test behaves as a large test that checks WriteSpan's behavior as a whole.
// Extra tests for individual functions are below.
func TestSpanWriter_WriteSpan(t *testing.T) {
	testCases := []struct {
		caption            string
		serviceIndexExists bool
		expectedError      string
		expectedLogs       []string
	}{
		{
			caption:            "span insertion error",
			serviceIndexExists: false,
			expectedError:      "",
			expectedLogs:       []string{"Wrote span to ES index"},
		},
	}
	for _, tc := range testCases {
		testCase := tc
		t.Run(testCase.caption, func(t *testing.T) {
			withSpanWriter(func(w *spanWriterTest) {
				date, err := time.Parse(time.RFC3339, "1995-04-21T22:08:41+00:00")
				require.NoError(t, err)

				span := &model.Span{
					TraceID:       model.NewTraceID(0, 1),
					SpanID:        model.NewSpanID(0),
					OperationName: "operation",
					Process: &model.Process{
						ServiceName: "service",
					},
					StartTime: date,
				}

				spanIndexName := "jaeger-span-1995-04-21"
				serviceIndexName := "jaeger-service-1995-04-21"
				serviceHash := "de3b5a8f1a79989d"

				indexService := &mocks.IndexService{}
				indexServicePut := &mocks.IndexService{}
				indexSpanPut := &mocks.IndexService{}

				indexService.On("Index", stringMatcher(spanIndexName)).Return(indexService)
				indexService.On("Index", stringMatcher(serviceIndexName)).Return(indexService)

				indexService.On("Type", stringMatcher(serviceType)).Return(indexServicePut)
				indexService.On("Type", stringMatcher(spanType)).Return(indexSpanPut)

				indexServicePut.On("Id", stringMatcher(serviceHash)).Return(indexServicePut)
				indexServicePut.On("BodyJson", mock.AnythingOfType("dbmodel.Service")).Return(indexServicePut)
				indexServicePut.On("Add")

				indexSpanPut.On("Id", mock.AnythingOfType("string")).Return(indexSpanPut)
				indexSpanPut.On("BodyJson", mock.AnythingOfType("**dbmodel.Span")).Return(indexSpanPut)
				indexSpanPut.On("Add")

				w.client.On("Index").Return(indexService)

				err = w.writer.WriteSpan(context.Background(), span)

				if testCase.expectedError == "" {
					require.NoError(t, err)
					indexServicePut.AssertNumberOfCalls(t, "Add", 1)
					indexSpanPut.AssertNumberOfCalls(t, "Add", 1)
				} else {
					require.EqualError(t, err, testCase.expectedError)
				}

				for _, expectedLog := range testCase.expectedLogs {
					assert.Contains(t, w.logBuffer.String(), expectedLog, "Log must contain %s, but was %s", expectedLog, w.logBuffer.String())
				}
				if len(testCase.expectedLogs) == 0 {
					assert.Equal(t, "", w.logBuffer.String())
				}
			})
		})
	}
}

func TestCreateTemplates(t *testing.T) {
	tests := []struct {
		err                    string
		spanTemplateService    func() *mocks.TemplateCreateService
		serviceTemplateService func() *mocks.TemplateCreateService
		indexPrefix            config.IndexPrefix
	}{
		{
			spanTemplateService: func() *mocks.TemplateCreateService {
				tService := &mocks.TemplateCreateService{}
				tService.On("Body", mock.Anything).Return(tService)
				tService.On("Do", context.Background()).Return(nil, nil)
				return tService
			},
			serviceTemplateService: func() *mocks.TemplateCreateService {
				tService := &mocks.TemplateCreateService{}
				tService.On("Body", mock.Anything).Return(tService)
				tService.On("Do", context.Background()).Return(nil, nil)
				return tService
			},
		},
		{
			spanTemplateService: func() *mocks.TemplateCreateService {
				tService := &mocks.TemplateCreateService{}
				tService.On("Body", mock.Anything).Return(tService)
				tService.On("Do", context.Background()).Return(nil, nil)
				return tService
			},
			serviceTemplateService: func() *mocks.TemplateCreateService {
				tService := &mocks.TemplateCreateService{}
				tService.On("Body", mock.Anything).Return(tService)
				tService.On("Do", context.Background()).Return(nil, nil)
				return tService
			},
			indexPrefix: "test",
		},
		{
			err: "span-template-error",
			spanTemplateService: func() *mocks.TemplateCreateService {
				tService := new(mocks.TemplateCreateService)
				tService.On("Body", mock.Anything).Return(tService)
				tService.On("Do", context.Background()).Return(nil, errors.New("span-template-error"))
				return tService
			},
			serviceTemplateService: func() *mocks.TemplateCreateService {
				tService := new(mocks.TemplateCreateService)
				tService.On("Body", mock.Anything).Return(tService)
				tService.On("Do", context.Background()).Return(nil, nil)
				return tService
			},
		},
		{
			err: "service-template-error",
			spanTemplateService: func() *mocks.TemplateCreateService {
				tService := new(mocks.TemplateCreateService)
				tService.On("Body", mock.Anything).Return(tService)
				tService.On("Do", context.Background()).Return(nil, nil)
				return tService
			},
			serviceTemplateService: func() *mocks.TemplateCreateService {
				tService := new(mocks.TemplateCreateService)
				tService.On("Body", mock.Anything).Return(tService)
				tService.On("Do", context.Background()).Return(nil, errors.New("service-template-error"))
				return tService
			},
		},
	}

	for _, test := range tests {
		withSpanWriter(func(w *spanWriterTest) {
			jaegerSpanId := test.indexPrefix.Apply("jaeger-span")
			jaegerServiceId := test.indexPrefix.Apply("jaeger-service")
			w.client.On("CreateTemplate", jaegerSpanId).Return(test.spanTemplateService())
			w.client.On("CreateTemplate", jaegerServiceId).Return(test.serviceTemplateService())
			err := w.writer.CreateTemplates(mock.Anything, mock.Anything, test.indexPrefix)
			if test.err != "" {
				require.Error(t, err, test.err)
			}
		})
	}
}

func TestSpanIndexName(t *testing.T) {
	date, err := time.Parse(time.RFC3339, "1995-04-21T22:08:41+00:00")
	require.NoError(t, err)
	span := &model.Span{
		StartTime: date,
	}
	spanIndexName := indexWithDate(spanIndexBaseName, "2006-01-02", span.StartTime)
	serviceIndexName := indexWithDate(serviceIndexBaseName, "2006-01-02", span.StartTime)
	assert.Equal(t, "jaeger-span-1995-04-21", spanIndexName)
	assert.Equal(t, "jaeger-service-1995-04-21", serviceIndexName)
}

func TestWriteSpanInternal(t *testing.T) {
	withSpanWriter(func(w *spanWriterTest) {
		indexService := &mocks.IndexService{}

		indexName := "jaeger-1995-04-21"
		indexService.On("Index", stringMatcher(indexName)).Return(indexService)
		indexService.On("Type", stringMatcher(spanType)).Return(indexService)
		indexService.On("BodyJson", mock.AnythingOfType("**dbmodel.Span")).Return(indexService)
		indexService.On("Add")

		w.client.On("Index").Return(indexService)

		jsonSpan := &dbmodel.Span{}

		w.writer.spanWriter.writeSpan(indexName, jsonSpan)
		indexService.AssertNumberOfCalls(t, "Add", 1)
		assert.Equal(t, "", w.logBuffer.String())
	})
}

func TestWriteSpanInternalError(t *testing.T) {
	withSpanWriter(func(w *spanWriterTest) {
		indexService := &mocks.IndexService{}

		indexName := "jaeger-1995-04-21"
		indexService.On("Index", stringMatcher(indexName)).Return(indexService)
		indexService.On("Type", stringMatcher(spanType)).Return(indexService)
		indexService.On("BodyJson", mock.AnythingOfType("**dbmodel.Span")).Return(indexService)
		indexService.On("Add")

		w.client.On("Index").Return(indexService)

		jsonSpan := &dbmodel.Span{
			TraceID: dbmodel.TraceID("1"),
			SpanID:  dbmodel.SpanID("0"),
		}

		w.writer.spanWriter.writeSpan(indexName, jsonSpan)
		indexService.AssertNumberOfCalls(t, "Add", 1)
	})
}

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
			mSpan := test.writer.spanWriter.spanConverter.FromDomainEmbedProcess(s)
			assert.Equal(t, test.expected.Tag, mSpan.Tag)
			assert.Equal(t, test.expected.Tags, mSpan.Tags)
			assert.Equal(t, test.expected.Process.Tag, mSpan.Process.Tag)
			assert.Equal(t, test.expected.Process.Tags, mSpan.Process.Tags)
		})
	}
}

func TestSpanWriterParamsTTL(t *testing.T) {
	logger, _ := testutils.NewLogger()
	metricsFactory := metricstest.NewFactory(0)
	testCases := []struct {
		serviceTTL       time.Duration
		name             string
		expectedAddCalls int
	}{
		{
			serviceTTL:       0,
			name:             "uses defaults",
			expectedAddCalls: 1,
		},
		{
			serviceTTL:       1 * time.Nanosecond,
			name:             "uses provided values",
			expectedAddCalls: 3,
		},
	}

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			client := &mocks.Client{}
			params := SpanWriterParams{
				Client:          func() es.Client { return client },
				Logger:          logger,
				MetricsFactory:  metricsFactory,
				ServiceCacheTTL: test.serviceTTL,
			}
			w := NewSpanWriterV1(params)

			svc := dbmodel.Service{
				ServiceName:   "foo",
				OperationName: "bar",
			}
			serviceHash := hashCode(svc)

			serviceIndexName := "jaeger-service-1995-04-21"

			indexService := &mocks.IndexService{}

			indexService.On("Index", stringMatcher(serviceIndexName)).Return(indexService)
			indexService.On("Type", stringMatcher(serviceType)).Return(indexService)
			indexService.On("Id", stringMatcher(serviceHash)).Return(indexService)
			indexService.On("BodyJson", mock.AnythingOfType("dbmodel.Service")).Return(indexService)
			indexService.On("Add")

			client.On("Index").Return(indexService)

			jsonSpan := &dbmodel.Span{
				Process:       dbmodel.Process{ServiceName: "foo"},
				OperationName: "bar",
			}

			w.spanWriter.writeService(serviceIndexName, jsonSpan)
			time.Sleep(1 * time.Nanosecond)
			w.spanWriter.writeService(serviceIndexName, jsonSpan)
			time.Sleep(1 * time.Nanosecond)
			w.spanWriter.writeService(serviceIndexName, jsonSpan)
			indexService.AssertNumberOfCalls(t, "Add", test.expectedAddCalls)
		})
	}
}

// stringMatcher can match a string argument when it contains a specific substring q
func stringMatcher(q string) any {
	matchFunc := func(s string) bool {
		return strings.Contains(s, q)
	}
	return mock.MatchedBy(matchFunc)
}
