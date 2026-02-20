// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package spanstore

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/internal/metrics"
	"github.com/jaegertracing/jaeger/internal/metricstest"
	es "github.com/jaegertracing/jaeger/internal/storage/elasticsearch"
	cfg "github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/dbmodel"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/mocks"
	"github.com/jaegertracing/jaeger/internal/storage/v1/api/spanstore"
	"github.com/jaegertracing/jaeger/internal/testutils"
)

type spanWriterTest struct {
	client    *mocks.Client
	logger    *zap.Logger
	logBuffer *testutils.Buffer
	writer    *SpanWriter
}

func withSpanWriter(fn func(w *spanWriterTest)) {
	client := &mocks.Client{}
	client.On("GetVersion").Return(uint(7))
	logger, logBuffer := testutils.NewLogger()
	metricsFactory := metricstest.NewFactory(0)
	w := &spanWriterTest{
		client:    client,
		logger:    logger,
		logBuffer: logBuffer,
		writer: NewSpanWriter(SpanWriterParams{
			Client: func() es.Client { return client },
			Logger: logger, MetricsFactory: metricsFactory,
			SpanIndex:    cfg.IndexOptions{DateLayout: "2006-01-02"},
			ServiceIndex: cfg.IndexOptions{DateLayout: "2006-01-02"},
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

	spanIndexOpts := cfg.IndexOptions{DateLayout: spanDataLayout}
	serviceIndexOpts := cfg.IndexOptions{DateLayout: serviceDataLayout}

	testCases := []struct {
		indices   []string
		params    SpanWriterParams
		esVersion uint
	}{
		{
			params: SpanWriterParams{
				Client: clientFn, Logger: logger, MetricsFactory: metricsFactory,
				SpanIndex: spanIndexOpts, ServiceIndex: serviceIndexOpts,
			},
			indices:   []string{spanIndexBaseName + spanDataLayoutFormat, serviceIndexBaseName + serviceDataLayoutFormat},
			esVersion: 7,
		},
		{
			params: SpanWriterParams{
				Client: clientFn, Logger: logger, MetricsFactory: metricsFactory,
				SpanIndex: spanIndexOpts, ServiceIndex: serviceIndexOpts, UseReadWriteAliases: true,
			},
			indices:   []string{spanIndexBaseName + "write", serviceIndexBaseName + "write"},
			esVersion: 7,
		},
		{
			params: SpanWriterParams{
				Client: clientFn, Logger: logger, MetricsFactory: metricsFactory,
				SpanIndex: spanIndexOpts, ServiceIndex: serviceIndexOpts,
				WriteAliasSuffix: "archive", // ignored because UseReadWriteAliases is false
			},
			indices:   []string{spanIndexBaseName + spanDataLayoutFormat, serviceIndexBaseName + serviceDataLayoutFormat},
			esVersion: 7,
		},
		{
			params: SpanWriterParams{
				Client: clientFn, Logger: logger, MetricsFactory: metricsFactory,
				SpanIndex: spanIndexOpts, ServiceIndex: serviceIndexOpts, IndexPrefix: "foo:",
			},
			indices:   []string{"foo:" + cfg.IndexPrefixSeparator + spanIndexBaseName + spanDataLayoutFormat, "foo:" + cfg.IndexPrefixSeparator + serviceIndexBaseName + serviceDataLayoutFormat},
			esVersion: 7,
		},
		{
			params: SpanWriterParams{
				Client: clientFn, Logger: logger, MetricsFactory: metricsFactory,
				SpanIndex: spanIndexOpts, ServiceIndex: serviceIndexOpts, IndexPrefix: "foo:", UseReadWriteAliases: true,
			},
			indices:   []string{"foo:-" + spanIndexBaseName + "write", "foo:-" + serviceIndexBaseName + "write"},
			esVersion: 7,
		},
		{
			params: SpanWriterParams{
				Client: clientFn, Logger: logger, MetricsFactory: metricsFactory,
				SpanIndex: spanIndexOpts, ServiceIndex: serviceIndexOpts, WriteAliasSuffix: "archive", UseReadWriteAliases: true,
			},
			indices:   []string{spanIndexBaseName + "archive", serviceIndexBaseName + "archive"},
			esVersion: 7,
		},
		{
			params: SpanWriterParams{
				Client: clientFn, Logger: logger, MetricsFactory: metricsFactory,
				SpanIndex: spanIndexOpts, ServiceIndex: serviceIndexOpts, IndexPrefix: "foo:", WriteAliasSuffix: "archive", UseReadWriteAliases: true,
			},
			indices:   []string{"foo:" + cfg.IndexPrefixSeparator + spanIndexBaseName + "archive", "foo:" + cfg.IndexPrefixSeparator + serviceIndexBaseName + "archive"},
			esVersion: 7,
		},
		{
			params: SpanWriterParams{
				Client: clientFn, Logger: logger, MetricsFactory: metricsFactory,
				SpanIndex: spanIndexOpts, ServiceIndex: serviceIndexOpts,
				UseReadWriteAliases: true,
				SpanWriteAlias:      "custom-span-write-alias", ServiceWriteAlias: "custom-service-write-alias",
			},
			indices:   []string{"custom-span-write-alias", "custom-service-write-alias"},
			esVersion: 7,
		},
		{
			params: SpanWriterParams{
				Client: clientFn, Logger: logger, MetricsFactory: metricsFactory,
				SpanIndex: spanIndexOpts, ServiceIndex: serviceIndexOpts,
				UseReadWriteAliases: true,
				SpanWriteAlias:      "custom-span-write-alias",
				ServiceWriteAlias:   "custom-service-write-alias",
				WriteAliasSuffix:    "archive", // Ignored when explicit aliases are used
			},
			indices:   []string{"custom-span-write-alias", "custom-service-write-alias"},
			esVersion: 7,
		},
		{
			params: SpanWriterParams{
				Client: clientFn, Logger: logger, MetricsFactory: metricsFactory,
				SpanIndex: spanIndexOpts, ServiceIndex: serviceIndexOpts, IndexPrefix: "foo:",
				UseReadWriteAliases: true,
				SpanWriteAlias:      "production-traces-write",
				ServiceWriteAlias:   "production-services-write",
			},
			indices:   []string{"production-traces-write", "production-services-write"},
			esVersion: 7,
		},
		{
			params: SpanWriterParams{
				Client: clientFn, Logger: logger, MetricsFactory: metricsFactory,
				SpanIndex: spanIndexOpts, ServiceIndex: serviceIndexOpts,
				UseDataStream: true,
			},
			indices:   []string{"jaeger-span-ds", serviceIndexBaseName + serviceDataLayoutFormat},
			esVersion: 8,
		},
		{
			params: SpanWriterParams{
				Client: clientFn, Logger: logger, MetricsFactory: metricsFactory,
				SpanIndex: spanIndexOpts, ServiceIndex: serviceIndexOpts, IndexPrefix: "foo:",
				UseDataStream: true,
			},
			indices:   []string{"foo:-jaeger-span-ds", "foo:-" + serviceIndexBaseName + serviceDataLayoutFormat},
			esVersion: 8,
		},
	}
	for _, testCase := range testCases {
		client := &mocks.Client{}
		client.On("GetVersion").Return(testCase.esVersion)
		testCase.params.Client = func() es.Client { return client }

		w := NewSpanWriter(testCase.params)
		spanIndexName, serviceIndexName := w.spanServiceIndex(date)
		assert.Equal(t, []string{spanIndexName, serviceIndexName}, testCase.indices)
	}
}

func TestClientClose(t *testing.T) {
	withSpanWriter(func(w *spanWriterTest) {
		w.client.On("GetVersion").Return(uint(7))
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
		esVersion          uint
	}{
		{
			caption:            "span insertion error",
			serviceIndexExists: false,
			expectedError:      "",
			expectedLogs:       []string{"Wrote span to ES index"},
		},
		{
			caption:            "ES 8 span insertion",
			serviceIndexExists: false,
			expectedError:      "",
			expectedLogs:       []string{"Wrote span to ES index"},
			esVersion:          8,
		},
	}
	for _, tc := range testCases {
		testCase := tc
		t.Run(testCase.caption, func(t *testing.T) {
			withSpanWriter(func(w *spanWriterTest) {
				esVersion := uint(7)
				if testCase.esVersion != 0 {
					esVersion = testCase.esVersion
				}
				w.client.On("GetVersion").Return(esVersion)

				date, err := time.Parse(time.RFC3339, "1995-04-21T22:08:41+00:00")
				require.NoError(t, err)

				span := &dbmodel.Span{
					TraceID:       "testing-traceid",
					SpanID:        "testing-spanid",
					OperationName: "operation",
					Process: dbmodel.Process{
						ServiceName: "service",
					},
					StartTime: model.TimeAsEpochMicroseconds(date),
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
				indexServicePut.On("Add", mock.Anything)

				indexSpanPut.On("Id", mock.AnythingOfType("string")).Return(indexSpanPut)
				indexSpanPut.On("BodyJson", mock.AnythingOfType("**dbmodel.Span")).Return(indexSpanPut)
				indexSpanPut.On("Add", mock.Anything)

				w.client.On("Index").Return(indexService)

				w.writer.WriteSpan(date, span)

				if testCase.expectedError == "" {
					indexServicePut.AssertNumberOfCalls(t, "Add", 1)
					indexSpanPut.AssertNumberOfCalls(t, "Add", 1)
				} else {
					require.EqualError(t, err, testCase.expectedError)
				}

				for _, expectedLog := range testCase.expectedLogs {
					assert.Contains(t, w.logBuffer.String(), expectedLog, "Log must contain %s, but was %s", expectedLog, w.logBuffer.String())
				}
				if len(testCase.expectedLogs) == 0 {
					assert.Empty(t, w.logBuffer.String())
				}
			})
		})
	}
}

func TestSpanIndexName(t *testing.T) {
	date, err := time.Parse(time.RFC3339, "1995-04-21T22:08:41+00:00")
	require.NoError(t, err)
	span := &model.Span{
		StartTime: date,
	}
	spanIndexName := cfg.IndexWithDate(spanIndexBaseName, "2006-01-02", span.StartTime)
	serviceIndexName := cfg.IndexWithDate(serviceIndexBaseName, "2006-01-02", span.StartTime)
	assert.Equal(t, "jaeger-span-1995-04-21", spanIndexName)
	assert.Equal(t, "jaeger-service-1995-04-21", serviceIndexName)
}

func TestWriteSpanInternal(t *testing.T) {
	testCases := []struct {
		caption   string
		esVersion uint
	}{
		{caption: "ES 7 version", esVersion: 7},
		{caption: "ES 8 version", esVersion: 8},
	}
	for _, tc := range testCases {
		testCase := tc
		t.Run(testCase.caption, func(t *testing.T) {
			withSpanWriter(func(w *spanWriterTest) {
				w.client.On("GetVersion").Return(testCase.esVersion)
				indexService := &mocks.IndexService{}

				indexName := "jaeger-1995-04-21"
				indexService.On("Index", stringMatcher(indexName)).Return(indexService)
				indexService.On("Type", stringMatcher(spanType)).Return(indexService)
				indexService.On("BodyJson", mock.AnythingOfType("**dbmodel.Span")).Return(indexService)
				indexService.On("Add", mock.Anything).Return(indexService)

				w.client.On("Index").Return(indexService)

				jsonSpan := &dbmodel.Span{}

				w.writer.writeSpanToIndex(indexName, jsonSpan)
				indexService.AssertNumberOfCalls(t, "Add", 1)
				assert.Empty(t, w.logBuffer.String())
			})
		})
	}
}

func TestWriteSpanInternalError(t *testing.T) {
	withSpanWriter(func(w *spanWriterTest) {
		indexService := &mocks.IndexService{}

		indexName := "jaeger-1995-04-21"
		indexService.On("Index", stringMatcher(indexName)).Return(indexService)
		indexService.On("Type", stringMatcher(spanType)).Return(indexService)
		indexService.On("BodyJson", mock.AnythingOfType("**dbmodel.Span")).Return(indexService)
		indexService.On("Add", mock.Anything)

		w.client.On("Index").Return(indexService)

		jsonSpan := &dbmodel.Span{
			TraceID: dbmodel.TraceID("1"),
			SpanID:  dbmodel.SpanID("0"),
		}

		w.writer.writeSpanToIndex(indexName, jsonSpan)
		indexService.AssertNumberOfCalls(t, "Add", 1)
	})
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
			client.On("GetVersion").Return(uint(7))
			params := SpanWriterParams{
				Client:          func() es.Client { return client },
				Logger:          logger,
				MetricsFactory:  metricsFactory,
				ServiceCacheTTL: test.serviceTTL,
			}
			w := NewSpanWriter(params)

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
			indexService.On("Add", mock.Anything)

			client.On("Index").Return(indexService)

			jsonSpan := &dbmodel.Span{
				Process:       dbmodel.Process{ServiceName: "foo"},
				OperationName: "bar",
			}

			w.writeService(serviceIndexName, jsonSpan)
			time.Sleep(1 * time.Nanosecond)
			w.writeService(serviceIndexName, jsonSpan)
			time.Sleep(1 * time.Nanosecond)
			w.writeService(serviceIndexName, jsonSpan)
			indexService.AssertNumberOfCalls(t, "Add", test.expectedAddCalls)
		})
	}
}

func TestTagMap(t *testing.T) {
	tags := []dbmodel.KeyValue{
		{
			Key:   "foo",
			Value: "foo",
			Type:  dbmodel.StringType,
		},
		{
			Key:   "a",
			Value: true,
			Type:  dbmodel.BoolType,
		},
		{
			Key:   "b.b",
			Value: int64(1),
			Type:  dbmodel.Int64Type,
		},
	}
	dbSpan := dbmodel.Span{Tags: tags, Process: dbmodel.Process{Tags: tags}}
	client := &mocks.Client{}
	client.On("GetVersion").Return(uint(7))
	converter := NewSpanWriter(SpanWriterParams{
		Client:            func() es.Client { return client },
		Logger:            zap.NewNop(),
		MetricsFactory:    metrics.NullFactory,
		AllTagsAsFields:   false,
		TagKeysAsFields:   []string{"a", "b.b", "b*"},
		TagDotReplacement: ":",
	})
	converter.convertNestedTagsToFieldTags(&dbSpan)

	assert.Len(t, dbSpan.Tags, 1)
	assert.Equal(t, "foo", dbSpan.Tags[0].Key)
	assert.Len(t, dbSpan.Process.Tags, 1)
	assert.Equal(t, "foo", dbSpan.Process.Tags[0].Key)

	tagsMap := map[string]any{}
	tagsMap["a"] = true
	tagsMap["b:b"] = int64(1)
	assert.Equal(t, tagsMap, dbSpan.Tag)
	assert.Equal(t, tagsMap, dbSpan.Process.Tag)
}

func TestNewSpanTags(t *testing.T) {
	testCases := []struct {
		params   SpanWriterParams
		expected dbmodel.Span
		name     string
	}{
		{
			params: SpanWriterParams{
				AllTagsAsFields:   true,
				TagKeysAsFields:   []string{},
				TagDotReplacement: "",
			},
			expected: dbmodel.Span{
				Tag: map[string]any{"foo": "bar"}, Tags: []dbmodel.KeyValue{},
				Process: dbmodel.Process{Tag: map[string]any{"bar": "baz"}, Tags: []dbmodel.KeyValue{}},
			},
			name: "allTagsAsFields",
		},
		{
			params: SpanWriterParams{
				AllTagsAsFields:   false,
				TagKeysAsFields:   []string{"foo", "bar", "rere"},
				TagDotReplacement: "",
			},
			expected: dbmodel.Span{
				Tag: map[string]any{"foo": "bar"}, Tags: []dbmodel.KeyValue{},
				Process: dbmodel.Process{Tag: map[string]any{"bar": "baz"}, Tags: []dbmodel.KeyValue{}},
			},
			name: "definedTagNames",
		},
		{
			params: SpanWriterParams{
				AllTagsAsFields:   false,
				TagKeysAsFields:   []string{},
				TagDotReplacement: "",
			},
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
	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			mSpan := &dbmodel.Span{
				Tags:    []dbmodel.KeyValue{{Key: "foo", Value: "bar", Type: dbmodel.StringType}},
				Process: dbmodel.Process{Tags: []dbmodel.KeyValue{{Key: "bar", Value: "baz", Type: dbmodel.StringType}}},
			}
			params := test.params
			client := &mocks.Client{}
			client.On("GetVersion").Return(uint(7))
			params.Client = func() es.Client { return client }
			params.Logger = zap.NewNop()
			params.MetricsFactory = metrics.NullFactory
			writer := NewSpanWriter(params)
			writer.convertNestedTagsToFieldTags(mSpan)
			assert.Equal(t, test.expected.Tag, mSpan.Tag)
			assert.Equal(t, test.expected.Tags, mSpan.Tags)
			assert.Equal(t, test.expected.Process.Tag, mSpan.Process.Tag)
			assert.Equal(t, test.expected.Process.Tags, mSpan.Process.Tags)
		})
	}
}

func TestNewSpanWriterV1(t *testing.T) {
	client := &mocks.Client{}
	client.On("GetVersion").Return(uint(7))
	params := SpanWriterParams{
		Client:         func() es.Client { return client },
		Logger:         zap.NewNop(),
		MetricsFactory: metricstest.NewFactory(0),
	}
	w := NewSpanWriterV1(params)
	assert.NotNil(t, w)

	span := &model.Span{
		TraceID: model.NewTraceID(1, 2),
		SpanID:  model.NewSpanID(3),
		Process: &model.Process{ServiceName: "foo"},
	}

	indexService := &mocks.IndexService{}
	indexService.On("Index", mock.MatchedBy(func(_ string) bool { return true })).Return(indexService)
	indexService.On("Type", mock.MatchedBy(func(_ string) bool { return true })).Return(indexService)
	indexService.On("Id", mock.MatchedBy(func(_ string) bool { return true })).Return(indexService)
	indexService.On("BodyJson", mock.Anything).Return(indexService)
	indexService.On("Add", mock.Anything)
	client.On("Index").Return(indexService)

	err := w.WriteSpan(context.Background(), span)
	require.NoError(t, err)

	client.On("Close").Return(nil)
	err = w.Close()
	require.NoError(t, err)
}

// stringMatcher can match a string argument when it contains a specific substring q
func stringMatcher(q string) any {
	matchFunc := func(s string) bool {
		return strings.Contains(s, q)
	}
	return mock.MatchedBy(matchFunc)
}

func TestDeprecatedIndexWithDate(t *testing.T) {
	// this test covers the deprecated function in index_utils.go to ensure code coverage
	date := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	name := indexWithDate("jaeger-span-", "2006-01-02", date)
	assert.Equal(t, "jaeger-span-2025-01-01", name)
}
