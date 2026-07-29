// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	"github.com/jaegertracing/jaeger/internal/metrics"
	"github.com/jaegertracing/jaeger/internal/metricstest"
	es "github.com/jaegertracing/jaeger/internal/storage/elasticsearch"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/esclient"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/indices"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/snapshottest"
	"github.com/jaegertracing/jaeger/internal/storage/v2/elasticsearch/tracestore/core/dbmodel"
	"github.com/jaegertracing/jaeger/internal/testutils"
)

// fakeBatchWriter records every batch handed to WriteBatch (flattened into items)
// and returns a configurable error, standing in for the esclient bulk writers in
// unit tests.
type fakeBatchWriter struct {
	items   []esclient.BulkItem
	batches int
	err     error
}

func (f *fakeBatchWriter) WriteBatch(_ context.Context, items []esclient.BulkItem) error {
	f.batches++
	f.items = append(f.items, items...)
	return f.err
}

type spanWriterTest struct {
	batchWriter *fakeBatchWriter
	added       *[]esclient.BulkItem
	logger      *zap.Logger
	logBuffer   *testutils.Buffer
	writer      *SpanWriter
}

func withSpanWriter(fn func(w *spanWriterTest)) {
	batchWriter := &fakeBatchWriter{}
	logger, logBuffer := testutils.NewLogger()
	metricsFactory := metricstest.NewFactory(0)
	w := &spanWriterTest{
		batchWriter: batchWriter,
		added:       &batchWriter.items,
		logger:      logger,
		logBuffer:   logBuffer,
		writer: NewSpanWriter(SpanWriterParams{
			BatchWriter:     batchWriter,
			Logger:          logger,
			MetricsFactory:  metricsFactory,
			SpanRotation:    indices.NewPeriodicRotation(config.SpanIndexName, "2006-01-02", 24*time.Hour),
			ServiceRotation: indices.NewPeriodicRotation(config.ServiceIndexName, "2006-01-02", 24*time.Hour),
		}),
	}
	fn(w)
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

// TestSpanWriter_WriteSpan checks WriteSpans's behavior as a whole: it assembles a
// new service:operation document and the span document for the batch and writes them
// through the batch writer. Extra tests for individual functions are below.
func TestSpanWriter_WriteSpan(t *testing.T) {
	withSpanWriter(func(w *spanWriterTest) {
		date, err := time.Parse(time.RFC3339, "1995-04-21T22:08:41+00:00")
		require.NoError(t, err)

		span := dbmodel.Span{
			TraceID:       "testing-traceid",
			SpanID:        "testing-spanid",
			OperationName: "operation",
			Process:       dbmodel.Process{ServiceName: "service"},
			StartTime:     model.TimeAsEpochMicroseconds(date),
		}

		require.NoError(t, w.writer.WriteSpans(context.Background(), []dbmodel.Span{span}))

		// One new service:operation doc, then the span doc.
		require.Len(t, *w.added, 2)
		service, spanItem := (*w.added)[0], (*w.added)[1]
		assert.Equal(t, "jaeger-service-1995-04-21", service.Index)
		assert.Equal(t, "de3b5a8f1a79989d", service.ID)
		assert.IsType(t, dbmodel.Service{}, service.Body)
		assert.Equal(t, "jaeger-span-1995-04-21", spanItem.Index)
		assert.Equal(t, es.WriteOpIndex, spanItem.OpType)
		// The span carries a deterministic _id (RFC 0007 §4.7) — the trace and span
		// IDs plus a content hash — so a re-sent identical span upserts.
		assert.Contains(t, spanItem.ID, "testing-traceid_testing-spanid_")
		// The span body is handed to the batch writer pre-encoded (marshaled once).
		assert.IsType(t, json.RawMessage{}, spanItem.Body)
		assert.Empty(t, w.logBuffer.String())

		// The service:operation pair is cached after the durable flush, so a repeat
		// batch re-sends only the span doc, not the service doc.
		require.NoError(t, w.writer.WriteSpans(context.Background(), []dbmodel.Span{span}))
		require.Len(t, *w.added, 3)
	})
}

// TestSpanWriter_WriteSpans covers the batch entry point: every span (and its
// service:operation pair) is enqueued, the per-span start time drives the index
// target, and the async enqueue reports no error.
func TestSpanWriter_WriteSpans(t *testing.T) {
	withSpanWriter(func(w *spanWriterTest) {
		date, err := time.Parse(time.RFC3339, "1995-04-21T22:08:41+00:00")
		require.NoError(t, err)
		startTime := model.TimeAsEpochMicroseconds(date)
		spans := []dbmodel.Span{
			{
				TraceID:       "trace-1",
				SpanID:        "span-1",
				OperationName: "op-1",
				Process:       dbmodel.Process{ServiceName: "service-a"},
				StartTime:     startTime,
			},
			{
				TraceID:       "trace-2",
				SpanID:        "span-2",
				OperationName: "op-2",
				Process:       dbmodel.Process{ServiceName: "service-b"},
				StartTime:     startTime,
			},
		}

		require.NoError(t, w.writer.WriteSpans(context.Background(), spans))

		// Two spans, each with a distinct service:operation, produce two service
		// documents and two span documents.
		require.Len(t, *w.added, 4)
		for _, item := range *w.added {
			assert.Contains(t, item.Index, "1995-04-21")
		}
	})
}

// TestSpanWriter_WriteSpansDedupsService pins that spans sharing a (service,
// operation) within one batch produce a single service:operation document — the
// service cache is confirmed only after the batch write, so the dedup must happen
// within the batch, not rely on the cache.
func TestSpanWriter_WriteSpansDedupsService(t *testing.T) {
	withSpanWriter(func(w *spanWriterTest) {
		startTime := model.TimeAsEpochMicroseconds(time.Now())
		spans := []dbmodel.Span{
			{TraceID: "t", SpanID: "a", OperationName: "op", Process: dbmodel.Process{ServiceName: "svc"}, StartTime: startTime},
			{TraceID: "t", SpanID: "b", OperationName: "op", Process: dbmodel.Process{ServiceName: "svc"}, StartTime: startTime},
			{TraceID: "t", SpanID: "c", OperationName: "op", Process: dbmodel.Process{ServiceName: "svc"}, StartTime: startTime},
		}
		require.NoError(t, w.writer.WriteSpans(context.Background(), spans))

		// Three span docs, but only one service:operation doc for the shared pair.
		assert.Equal(t, 1, countServiceDocs(*w.added), "the shared service doc is written once")
		assert.Len(t, *w.added, 4)
	})
}

// TestSpanWriter_WriteSpansEmpty pins that an empty batch is a no-op that still
// reports success.
func TestSpanWriter_WriteSpansEmpty(t *testing.T) {
	withSpanWriter(func(w *spanWriterTest) {
		require.NoError(t, w.writer.WriteSpans(context.Background(), nil))
		assert.Empty(t, *w.added)
	})
}

func newSpanWriterWith(bw esclient.BatchWriter) *SpanWriter {
	logger, _ := testutils.NewLogger()
	return NewSpanWriter(SpanWriterParams{
		BatchWriter:     bw,
		Logger:          logger,
		MetricsFactory:  metricstest.NewFactory(0),
		SpanRotation:    indices.NewAliasedRotation("jaeger-span-write", "jaeger-span-read"),
		ServiceRotation: indices.NewAliasedRotation("jaeger-service-write", "jaeger-service-read"),
	})
}

// TestSpanWriter_BatchWrite covers the WriteSpans contract independent of write mode:
// the whole batch is handed to the batch writer in one call, its error propagates,
// and the service cache is updated only after a successful (durable) write — so a
// failed-and-retried batch re-sends the service doc (RFC 0007 §4.3).
func TestSpanWriter_BatchWrite(t *testing.T) {
	spanA := dbmodel.Span{TraceID: "1", SpanID: "a", OperationName: "op", Process: dbmodel.Process{ServiceName: "svc"}}
	spanB := dbmodel.Span{TraceID: "1", SpanID: "b", OperationName: "op2", Process: dbmodel.Process{ServiceName: "svc"}}

	t.Run("one batch write, all items", func(t *testing.T) {
		fake := &fakeBatchWriter{}
		writer := newSpanWriterWith(fake)
		require.NoError(t, writer.WriteSpans(context.Background(), []dbmodel.Span{spanA, spanB}))
		assert.Equal(t, 1, fake.batches, "the whole batch is one WriteBatch call")
		// two new service:operation docs + two span docs
		assert.Len(t, fake.items, 4)
	})

	t.Run("error propagates", func(t *testing.T) {
		fake := &fakeBatchWriter{err: errors.New("bulk rejected")}
		writer := newSpanWriterWith(fake)
		err := writer.WriteSpans(context.Background(), []dbmodel.Span{spanA})
		require.ErrorContains(t, err, "bulk rejected")
	})

	t.Run("service cache updated only after durable write", func(t *testing.T) {
		fake := &fakeBatchWriter{err: errors.New("boom")}
		writer := newSpanWriterWith(fake)

		// First write fails, so the service doc must NOT be cached.
		require.Error(t, writer.WriteSpans(context.Background(), []dbmodel.Span{spanA}))
		firstBatchItems := len(fake.items)
		fake.err = nil
		require.NoError(t, writer.WriteSpans(context.Background(), []dbmodel.Span{spanA}))

		// The retry re-sends the service doc precisely because the first write was
		// not durable (RFC 0007 §4.3) — it was not cached.
		assert.Equal(t, 1, countServiceDocs(fake.items[firstBatchItems:]),
			"service doc re-sent after a failed write")
	})
}

func countServiceDocs(items []esclient.BulkItem) int {
	n := 0
	for _, item := range items {
		if _, ok := item.Body.(dbmodel.Service); ok {
			n++
		}
	}
	return n
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

func TestBuildSpanItem(t *testing.T) {
	withSpanWriter(func(w *spanWriterTest) {
		item, err := w.writer.buildSpanItem("jaeger-1995-04-21", &dbmodel.Span{})

		require.NoError(t, err)
		assert.Equal(t, "jaeger-1995-04-21", item.Index)
		assert.Equal(t, es.WriteOpIndex, item.OpType)
		assert.IsType(t, json.RawMessage{}, item.Body)
	})
}

func TestBuildSpanItem_DataStreamOpType(t *testing.T) {
	// A data stream rotation must drive the bulk op type to "create" (append-only)
	// rather than the legacy "index".
	logger, _ := testutils.NewLogger()
	writer := NewSpanWriter(SpanWriterParams{
		Logger:          logger,
		MetricsFactory:  metricstest.NewFactory(0),
		SpanRotation:    indices.NewDataStreamRotation("jaeger.spans", ""),
		ServiceRotation: indices.NewPeriodicRotation(config.ServiceIndexName, "2006-01-02", 24*time.Hour),
	})

	item, err := writer.buildSpanItem("jaeger.spans", &dbmodel.Span{})

	require.NoError(t, err)
	assert.Equal(t, es.WriteOpCreate, item.OpType)
	assert.Equal(t, "jaeger.spans", item.Index)
}

func TestBuildSpanItem_EncodeError(t *testing.T) {
	writer := NewSpanWriter(SpanWriterParams{
		Logger:          zap.NewNop(),
		MetricsFactory:  metricstest.NewFactory(0),
		SpanRotation:    indices.NewAliasedRotation("jaeger-span-write", "jaeger-span-read"),
		ServiceRotation: indices.NewAliasedRotation("jaeger-service-write", "jaeger-service-read"),
	})

	// A NaN float tag value cannot be JSON-encoded, so buildSpanItem returns an error
	// (the caller decides to drop the span — see TestSpanWriter_WriteSpansEncodeError).
	span := &dbmodel.Span{TraceID: "a", SpanID: "b", Tag: map[string]any{"bad": math.NaN()}}
	item, err := writer.buildSpanItem("jaeger-span-write", span)

	require.Error(t, err)
	assert.Empty(t, item.ID)
}

// TestSpanWriter_WriteSpansEncodeError pins that WriteSpans drops an unencodable span
// (logging the error) instead of failing the whole batch.
func TestSpanWriter_WriteSpansEncodeError(t *testing.T) {
	withSpanWriter(func(w *spanWriterTest) {
		startTime := model.TimeAsEpochMicroseconds(time.Now())
		good := dbmodel.Span{TraceID: "t", SpanID: "ok", OperationName: "op", Process: dbmodel.Process{ServiceName: "svc"}, StartTime: startTime}
		// A NaN tag value (kept in Tags, so it survives tag elevation) makes this
		// span fail to JSON-encode. It shares (service, op) with the good span so the
		// service doc is deduped.
		bad := dbmodel.Span{
			TraceID: "t", SpanID: "bad", OperationName: "op",
			Process:   dbmodel.Process{ServiceName: "svc"},
			Tags:      []dbmodel.KeyValue{{Key: "x", Type: dbmodel.Float64Type, Value: math.NaN()}},
			StartTime: startTime,
		}

		require.NoError(t, w.writer.WriteSpans(context.Background(), []dbmodel.Span{good, bad}))
		assert.Contains(t, w.logBuffer.String(), "failed to encode span document")
		// The shared service doc and the good span's doc are written; the bad span is
		// dropped (its service doc was deduped against the good span's).
		assert.Len(t, *w.added, 2)
	})
}

func TestSpanDocID(t *testing.T) {
	// id marshals the span the way the writer does, then derives the _id, so the
	// test exercises the real (marshal → hash) path.
	id := func(s *dbmodel.Span) string {
		body, err := json.Marshal(s)
		require.NoError(t, err)
		return spanDocID(s, body)
	}
	base := &dbmodel.Span{
		TraceID:       "1234567890abcdef",
		SpanID:        "abcdef1234567890",
		OperationName: "op",
		StartTime:     100,
	}

	// Deterministic: an identical span always hashes to the same id, so a re-sent
	// span upserts rather than duplicating.
	sameContent := *base
	assert.Equal(t, id(base), id(&sameContent))

	// Content-sensitive: a different field yields a different id.
	other := *base
	other.OperationName = "other-op"
	assert.NotEqual(t, id(base), id(&other), "differing content must not collide")

	// Shared-span model: a client and server span may legitimately share
	// (traceID, spanID) but differ in content; the content hash keeps them distinct
	// (a traceID+spanID key would wrongly collapse them).
	sharedID := *base
	sharedID.Tags = []dbmodel.KeyValue{{Key: "span.kind", Type: dbmodel.StringType, Value: "server"}}
	assert.NotEqual(t, id(base), id(&sharedID))

	// The id is scoped by trace and span id (delimited so it stays injective even
	// for empty ids): distinct (traceID, spanID) never collide, so a hash collision
	// can only ever affect spans that share them.
	assert.Contains(t, id(base), string(base.TraceID)+"_"+string(base.SpanID)+"_")
}

func TestWriteSpan_DataStreamTimestamp(t *testing.T) {
	date := time.Date(2024, time.June, 18, 10, 0, 0, 0, time.UTC)

	logger, _ := testutils.NewLogger()
	metricsFactory := metricstest.NewFactory(0)
	writer := NewSpanWriter(SpanWriterParams{
		BatchWriter:     &fakeBatchWriter{},
		Logger:          logger,
		MetricsFactory:  metricsFactory,
		SpanRotation:    indices.NewDataStreamRotation("jaeger.spans", ""),
		ServiceRotation: indices.NewDataStreamRotation("jaeger.services", ""),
	})

	spans := []dbmodel.Span{{TraceID: "abc", SpanID: "def", StartTime: model.TimeAsEpochMicroseconds(date)}}
	require.NoError(t, writer.WriteSpans(context.Background(), spans))

	// The data stream write path stamps @timestamp as epoch nanoseconds.
	assert.Equal(t, strconv.FormatInt(date.UnixNano(), 10), spans[0].Timestamp)
	out, err := json.Marshal(spans[0])
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
			batchWriter := &fakeBatchWriter{}
			added := &batchWriter.items
			params := SpanWriterParams{
				BatchWriter:     batchWriter,
				Logger:          logger,
				MetricsFactory:  metricsFactory,
				ServiceCacheTTL: test.serviceTTL,
				SpanRotation:    indices.NewPeriodicRotation(config.SpanIndexName, "2006-01-02", 24*time.Hour),
				ServiceRotation: indices.NewPeriodicRotation(config.ServiceIndexName, "2006-01-02", 24*time.Hour),
			}
			w := NewSpanWriter(params)

			span := dbmodel.Span{
				Process:       dbmodel.Process{ServiceName: "foo"},
				OperationName: "bar",
			}

			// Each WriteSpans also enqueues a span doc; the service doc is written only
			// when the (service, operation) pair is not cached, which the TTL governs.
			for range 3 {
				require.NoError(t, w.WriteSpans(context.Background(), []dbmodel.Span{span}))
				time.Sleep(1 * time.Nanosecond)
			}
			serviceDocs := 0
			for _, item := range *added {
				if _, ok := item.Body.(dbmodel.Service); ok {
					serviceDocs++
				}
			}
			assert.Equal(t, test.expectedAddCalls, serviceDocs)
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
			BatchWriter:     bulkWriter,
			Logger:          zap.NewNop(),
			MetricsFactory:  metrics.NullFactory,
			SpanRotation:    indices.NewAliasedRotation(writeIndex, "jaeger-span-read"),
			ServiceRotation: indices.NewAliasedRotation("jaeger-service-write-000001", "jaeger-service-read"),
		})

		rec.Reset()
		item, err := writer.buildSpanItem(writeIndex, span)
		require.NoError(t, err)
		bulkWriter.Add(item)
		require.NoError(t, bulkWriter.Close()) // flushes the bulk request
		writeSpan[version] = rec.Marshal(t)
	}

	snapshottest.AssertByVersion(t, "testdata/write_span", writeSpan)
}
