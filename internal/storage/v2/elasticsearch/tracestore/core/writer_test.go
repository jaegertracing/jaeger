// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strconv"
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
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/esclient"
	esclientmocks "github.com/jaegertracing/jaeger/internal/storage/elasticsearch/esclient/mocks"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/indices"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/snapshottest"
	"github.com/jaegertracing/jaeger/internal/storage/v2/elasticsearch/tracestore/core/dbmodel"
	"github.com/jaegertracing/jaeger/internal/testutils"
)

type spanWriterTest struct {
	bulkWriter *esclientmocks.BulkWriter
	added      *[]esclient.BulkItem
	logger     *zap.Logger
	logBuffer  *testutils.Buffer
	writer     *SpanWriter
}

func withSpanWriter(fn func(w *spanWriterTest)) {
	bulkWriter := &esclientmocks.BulkWriter{}
	added := captureBulk(bulkWriter)
	logger, logBuffer := testutils.NewLogger()
	metricsFactory := metricstest.NewFactory(0)
	w := &spanWriterTest{
		bulkWriter: bulkWriter,
		added:      added,
		logger:     logger,
		logBuffer:  logBuffer,
		writer: NewSpanWriter(SpanWriterParams{
			BulkWriter:      bulkWriter,
			Logger:          logger,
			MetricsFactory:  metricsFactory,
			SpanRotation:    indices.NewPeriodicRotation(config.SpanIndexName, "2006-01-02", 24*time.Hour),
			ServiceRotation: indices.NewPeriodicRotation(config.ServiceIndexName, "2006-01-02", 24*time.Hour),
		}),
	}
	fn(w)
}

// captureBulk programs a BulkWriter mock to record every Add, returning a
// pointer to the growing slice of items the writer enqueued.
func captureBulk(m *esclientmocks.BulkWriter) *[]esclient.BulkItem {
	items := &[]esclient.BulkItem{}
	m.On("Add", mock.AnythingOfType("esclient.BulkItem")).Run(func(args mock.Arguments) {
		*items = append(*items, args.Get(0).(esclient.BulkItem))
	}).Return()
	return items
}

func TestSpanWriterRotations(t *testing.T) {
	logger, _ := testutils.NewLogger()
	metricsFactory := metricstest.NewFactory(0)
	date := time.Date(2019, 10, 10, 5, 0, 0, 0, time.UTC)

	testCases := []struct {
		name            string
		spanRotation    indices.Rotation
		serviceRotation indices.Rotation
		expectedSpan    string
		expectedService string
	}{
		{
			name:            "periodic rotations",
			spanRotation:    indices.NewPeriodicRotation(config.SpanIndexName, "2006-01-02-15", 24*time.Hour),
			serviceRotation: indices.NewPeriodicRotation(config.ServiceIndexName, "2006-01-02", 24*time.Hour),
			expectedSpan:    "jaeger-span-2019-10-10-05",
			expectedService: "jaeger-service-2019-10-10",
		},
		{
			name:            "aliased rotations",
			spanRotation:    indices.NewAliasedRotation("jaeger-span-write", "jaeger-span-read"),
			serviceRotation: indices.NewAliasedRotation("jaeger-service-write", "jaeger-service-read"),
			expectedSpan:    "jaeger-span-write",
			expectedService: "jaeger-service-write",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			w := NewSpanWriter(SpanWriterParams{
				Logger:          logger,
				MetricsFactory:  metricsFactory,
				SpanRotation:    tc.spanRotation,
				ServiceRotation: tc.serviceRotation,
			})
			assert.Equal(t, tc.expectedSpan, w.spanRotation.WriteTarget(date))
			assert.Equal(t, tc.expectedService, w.serviceRotation.WriteTarget(date))
		})
	}
}

func TestSpanWriterClose(t *testing.T) {
	// The writer owns no resources; Close is a no-op (the factory owns the bulk
	// indexer). This just pins that contract.
	withSpanWriter(func(w *spanWriterTest) {
		require.NoError(t, w.writer.Close())
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

				w.writer.WriteSpan(date, span)

				require.Len(t, *w.added, 2)
				service, spanItem := (*w.added)[0], (*w.added)[1]
				assert.Equal(t, serviceIndexName, service.Index)
				assert.Equal(t, serviceHash, service.ID)
				assert.IsType(t, dbmodel.Service{}, service.Body)
				assert.Equal(t, spanIndexName, spanItem.Index)
				assert.Equal(t, es.WriteOpIndex, spanItem.OpType)

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
	spanIndexName := indices.IndexWithDate(config.SpanIndexName, "2006-01-02", span.StartTime)
	serviceIndexName := indices.IndexWithDate(config.ServiceIndexName, "2006-01-02", span.StartTime)
	assert.Equal(t, "jaeger-span-1995-04-21", spanIndexName)
	assert.Equal(t, "jaeger-service-1995-04-21", serviceIndexName)
}

func TestWriteSpanInternal(t *testing.T) {
	withSpanWriter(func(w *spanWriterTest) {
		indexName := "jaeger-1995-04-21"
		jsonSpan := &dbmodel.Span{}

		w.writer.writeSpanToIndex(indexName, jsonSpan)

		require.Len(t, *w.added, 1)
		item := (*w.added)[0]
		assert.Equal(t, indexName, item.Index)
		assert.Equal(t, es.WriteOpIndex, item.OpType)
		assert.Empty(t, w.logBuffer.String())
	})
}

func TestWriteSpanInternalError(t *testing.T) {
	withSpanWriter(func(w *spanWriterTest) {
		indexName := "jaeger-1995-04-21"
		jsonSpan := &dbmodel.Span{
			TraceID: dbmodel.TraceID("1"),
			SpanID:  dbmodel.SpanID("0"),
		}

		w.writer.writeSpanToIndex(indexName, jsonSpan)
		require.Len(t, *w.added, 1)
	})
}

func TestWriteSpanToIndex_DataStreamOpType(t *testing.T) {
	// A data stream rotation must drive the bulk op type to "create" (append-only)
	// rather than the legacy "index".
	logger, _ := testutils.NewLogger()
	metricsFactory := metricstest.NewFactory(0)
	bulkWriter := &esclientmocks.BulkWriter{}
	added := captureBulk(bulkWriter)
	writer := NewSpanWriter(SpanWriterParams{
		BulkWriter:      bulkWriter,
		Logger:          logger,
		MetricsFactory:  metricsFactory,
		SpanRotation:    indices.NewDataStreamRotation("jaeger.spans", ""),
		ServiceRotation: indices.NewPeriodicRotation(config.ServiceIndexName, "2006-01-02", 24*time.Hour),
	})

	writer.writeSpanToIndex("jaeger.spans", &dbmodel.Span{})

	require.Len(t, *added, 1)
	assert.Equal(t, es.WriteOpCreate, (*added)[0].OpType)
	assert.Equal(t, "jaeger.spans", (*added)[0].Index)
}

// noWriteRotation is a stub whose WriteTarget is empty, so WriteSpan skips the
// service write and we can assert on the span write in isolation.
type noWriteRotation struct{}

func (noWriteRotation) WriteTarget(time.Time) string              { return "" }
func (noWriteRotation) ReadTargets(time.Time, time.Time) []string { return nil }
func (noWriteRotation) WriteOpType() es.WriteOpType               { return es.WriteOpIndex }
func (noWriteRotation) RequiresDocumentTimestamp() bool           { return false }

func TestWriteSpan_DataStreamTimestamp(t *testing.T) {
	date := time.Date(2024, time.June, 18, 10, 0, 0, 0, time.UTC)

	logger, _ := testutils.NewLogger()
	metricsFactory := metricstest.NewFactory(0)
	bulkWriter := &esclientmocks.BulkWriter{}
	captureBulk(bulkWriter)
	writer := NewSpanWriter(SpanWriterParams{
		BulkWriter:      bulkWriter,
		Logger:          logger,
		MetricsFactory:  metricsFactory,
		SpanRotation:    indices.NewDataStreamRotation("jaeger.spans", ""),
		ServiceRotation: noWriteRotation{},
	})

	span := &dbmodel.Span{TraceID: "abc", SpanID: "def"}
	writer.WriteSpan(date, span)

	// The data stream write path stamps @timestamp as epoch nanoseconds.
	assert.Equal(t, strconv.FormatInt(date.UnixNano(), 10), span.Timestamp)
	out, err := json.Marshal(span)
	require.NoError(t, err)
	assert.Contains(t, string(out), `"@timestamp":"`+strconv.FormatInt(date.UnixNano(), 10)+`"`)
}

func TestWriteSpan_LegacyOmitsTimestamp(t *testing.T) {
	// Legacy (non-data-stream) writes must not emit @timestamp, keeping the
	// document schema unchanged.
	span := &dbmodel.Span{TraceID: "abc", SpanID: "def"}
	out, err := json.Marshal(span)
	require.NoError(t, err)
	assert.NotContains(t, string(out), "@timestamp")
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
			bulkWriter := &esclientmocks.BulkWriter{}
			added := captureBulk(bulkWriter)
			params := SpanWriterParams{
				BulkWriter:      bulkWriter,
				Logger:          logger,
				MetricsFactory:  metricsFactory,
				ServiceCacheTTL: test.serviceTTL,
			}
			w := NewSpanWriter(params)

			serviceIndexName := "jaeger-service-1995-04-21"
			jsonSpan := &dbmodel.Span{
				Process:       dbmodel.Process{ServiceName: "foo"},
				OperationName: "bar",
			}

			w.writeService(serviceIndexName, jsonSpan)
			time.Sleep(1 * time.Nanosecond)
			w.writeService(serviceIndexName, jsonSpan)
			time.Sleep(1 * time.Nanosecond)
			w.writeService(serviceIndexName, jsonSpan)
			assert.Len(t, *added, test.expectedAddCalls)
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
	converter := NewSpanWriter(SpanWriterParams{
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

// stringMatcher can match a string argument when it contains a specific substring q
func stringMatcher(q string) any {
	matchFunc := func(s string) bool {
		return strings.Contains(s, q)
	}
	return mock.MatchedBy(matchFunc)
}

// TestWriterRequestSnapshots freezes the wire format of the span bulk write.
func TestWriterRequestSnapshots(t *testing.T) {
	const writeIndex = "jaeger-span-write-000001"
	const startMicros = 1577934245000000
	span := &dbmodel.Span{
		TraceID:         "1234567890abcdef",
		SpanID:          "abcdef1234567890",
		OperationName:   "test-operation",
		StartTime:       startMicros,
		StartTimeMillis: startMicros / 1000, // derived from StartTime, per to_dbmodel.go
		Duration:        1000,
		Process:         dbmodel.Process{ServiceName: "test-service"},
	}

	writeSpan := map[es.BackendVersion]string{}
	for _, version := range es.AllVersions {
		rec := dataRecorder()
		server := httptest.NewServer(rec)
		t.Cleanup(server.Close)

		// A real esclient bulk indexer over the recording server; version is pinned
		// so the client skips its probe. The single span buffers until Close, which
		// flushes the one bulk request we record.
		esCfg := &config.Configuration{Servers: []string{server.URL}, Version: uint(version)}
		esClient, err := esclient.NewClient(context.Background(), esCfg, zap.NewNop(), nil)
		require.NoError(t, err)
		bulkWriter, err := esclient.NewBulkIndexer(esClient, esclient.BulkIndexerConfig{}, metrics.NullFactory, zap.NewNop())
		require.NoError(t, err)
		writer := NewSpanWriter(SpanWriterParams{
			BulkWriter:      bulkWriter,
			Logger:          zap.NewNop(),
			MetricsFactory:  metrics.NullFactory,
			SpanRotation:    indices.NewAliasedRotation(writeIndex, "jaeger-span-read"),
			ServiceRotation: indices.NewAliasedRotation("jaeger-service-write-000001", "jaeger-service-read"),
		})

		rec.Reset()
		writer.writeSpanToIndex(writeIndex, span)
		require.NoError(t, bulkWriter.Close()) // flushes the bulk request
		writeSpan[version] = rec.Marshal(t)
	}

	snapshottest.AssertByVersion(t, "testdata/write_span", writeSpan)
}
