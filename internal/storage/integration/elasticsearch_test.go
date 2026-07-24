// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/config/configoptional"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/internal/jiter"
	"github.com/jaegertracing/jaeger/internal/jptrace"
	"github.com/jaegertracing/jaeger/internal/metrics"
	esstorage "github.com/jaegertracing/jaeger/internal/storage/elasticsearch"
	escfg "github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
	"github.com/jaegertracing/jaeger/internal/storage/elasticsearch/esclient"
	"github.com/jaegertracing/jaeger/internal/storage/integration/capabilities"
	es "github.com/jaegertracing/jaeger/internal/storage/v1/elasticsearch"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/depstore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	esv2 "github.com/jaegertracing/jaeger/internal/storage/v2/elasticsearch"
	"github.com/jaegertracing/jaeger/internal/telemetry"
	"github.com/jaegertracing/jaeger/internal/testutils"
)

const (
	host               = "0.0.0.0"
	queryPort          = "9200"
	queryHostPort      = host + ":" + queryPort
	queryURL           = "http://" + queryHostPort
	indexPrefix        = "integration-test"
	indexDateLayout    = "2006-01-02"
	tagKeyDeDotChar    = "@"
	maxSpanAge         = time.Hour * 72
	defaultMaxDocCount = 10_000
	archiveAliasSuffix = "archive"
)

type ESStorageIntegration struct {
	StorageIntegration

	client *esTestClient

	ArchiveTraceReader tracestore.Reader
	ArchiveTraceWriter tracestore.Writer

	factory        *esv2.Factory
	archiveFactory *esv2.Factory

	// writeMode selects the elasticsearch.write_mode for the factories under test
	// (empty = async default; "sync" exercises the RFC 0007 synchronous path).
	writeMode escfg.WriteMode
}

func (s *ESStorageIntegration) initializeES(t *testing.T, allTagsAsFields bool) {
	s.client = newESTestClient(t)

	s.initSpanstore(t, allTagsAsFields)

	s.CleanUp = func(t *testing.T) {
		s.esCleanUp(t)
	}
	s.esCleanUp(t)
}

func (s *ESStorageIntegration) esCleanUp(t *testing.T) {
	require.NoError(t, s.factory.Purge(context.Background()))
	require.NoError(t, s.archiveFactory.Purge(context.Background()))
}

func (s *ESStorageIntegration) initSpanstore(t *testing.T, allTagsAsFields bool) {
	cfg := es.DefaultConfig()
	cfg.CreateIndexTemplates = true
	cfg.BulkProcessing = escfg.BulkProcessing{
		MaxBytes: 1, // flush on essentially every document, for test determinism
	}
	cfg.WriteMode = s.writeMode
	cfg.Tags.AllAsFields = allTagsAsFields
	cfg.ServiceCacheTTL = 1 * time.Second
	cfg.Indices.IndexPrefix = indexPrefix
	var err error
	f, err := esv2.NewFactory(context.Background(), cfg, telemetry.NoopSettings(), nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, f.Close())
	})
	acfg := es.DefaultConfig()
	acfg.ReadAliasSuffix = archiveAliasSuffix
	acfg.WriteAliasSuffix = archiveAliasSuffix
	acfg.UseReadWriteAliases = configoptional.Some(true)
	acfg.WriteMode = s.writeMode
	acfg.Tags.AllAsFields = allTagsAsFields
	acfg.Indices.IndexPrefix = indexPrefix
	af, err := esv2.NewFactory(context.Background(), acfg, telemetry.NoopSettings(), nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, af.Close())
	})
	s.factory = f
	s.archiveFactory = af
	s.TraceWriter, err = f.CreateTraceWriter()
	require.NoError(t, err)
	s.TraceReader, err = f.CreateTraceReader()
	require.NoError(t, err)
	s.ArchiveTraceReader, err = af.CreateTraceReader()
	require.NoError(t, err)
	s.ArchiveTraceWriter, err = af.CreateTraceWriter()
	require.NoError(t, err)
	s.DependencyReader, err = f.CreateDependencyReader()
	require.NoError(t, err)
	s.DependencyWriter = s.DependencyReader.(depstore.Writer)
	s.SamplingStore, err = f.CreateSamplingStore(1)
	require.NoError(t, err)
}

func healthCheck(c *http.Client) error {
	for range 200 {
		if resp, err := c.Get(queryURL); err == nil {
			return resp.Body.Close()
		}
		time.Sleep(100 * time.Millisecond)
	}
	return errors.New("elastic search is not ready")
}

func runElasticsearchTest(t *testing.T, allTagsAsFields bool, writeMode escfg.WriteMode) {
	SkipUnlessEnv(t, StorageElasticsearch, StorageOpenSearch)
	c := getESHttpClient(t)
	require.NoError(t, healthCheck(c))
	s := &ESStorageIntegration{
		StorageIntegration: StorageIntegration{
			Fixtures:     LoadAndParseQueryTestCases(t, "fixtures/queries_es.json"),
			Capabilities: capabilities.Elasticsearch(),
		},
		writeMode: writeMode,
	}
	s.initializeES(t, allTagsAsFields)
	s.RunAll(t)
	t.Run("ArchiveTrace", s.testArchiveTrace)
}

func TestElasticsearchStorage(t *testing.T) {
	t.Cleanup(func() {
		testutils.VerifyGoLeaksOnce(t)
	})
	runElasticsearchTest(t, false, escfg.WriteModeAsync)
}

func TestElasticsearchStorage_AllTagsAsObjectFields(t *testing.T) {
	t.Cleanup(func() {
		testutils.VerifyGoLeaksOnce(t)
	})
	runElasticsearchTest(t, true, escfg.WriteModeAsync)
}

// TestElasticsearchStorage_Sync runs the full trace-storage suite with
// elasticsearch.write_mode: sync, validating the wired synchronous write path
// (RFC 0007 M4) end-to-end against a live backend alongside the async run.
func TestElasticsearchStorage_Sync(t *testing.T) {
	t.Cleanup(func() {
		testutils.VerifyGoLeaksOnce(t)
	})
	runElasticsearchTest(t, false, escfg.WriteModeSync)
}

func TestElasticsearchStorage_IndexTemplates(t *testing.T) {
	SkipUnlessEnv(t, StorageElasticsearch, StorageOpenSearch)
	t.Cleanup(func() {
		testutils.VerifyGoLeaksOnce(t)
	})
	c := getESHttpClient(t)
	require.NoError(t, healthCheck(c))
	s := &ESStorageIntegration{}
	s.initializeES(t, true)
	// templateExists picks the composable (_index_template) or legacy (_template)
	// API by backend version, so this assertion is uniform across ES 7–9 / OS.
	assert.True(t, s.client.templateExists(t, indexPrefix+"-jaeger-service"))
	assert.True(t, s.client.templateExists(t, indexPrefix+"-jaeger-span"))
	s.cleanESIndexTemplates(t, indexPrefix)
}

func (s *ESStorageIntegration) cleanESIndexTemplates(t *testing.T, prefix string) {
	s.client.cleanTemplates(t, prefix)
}

// TestElasticsearchStorage_SyncBulkWriter exercises the RFC 0007 synchronous bulk
// primitive against a live backend: it durably writes documents in one blocking
// _bulk round-trip, reads them back to prove durability, and forces a real
// item-level rejection to prove the error propagates (unlike the async indexer,
// whose failures never reach the caller). This runs in the existing ES/OS matrix
// job, so the primitive is validated against real ES 7–9 / OS 1–3 the milestone
// it lands, not only via httptest mocks.
func TestElasticsearchStorage_SyncBulkWriter(t *testing.T) {
	SkipUnlessEnv(t, StorageElasticsearch, StorageOpenSearch)
	t.Cleanup(func() {
		testutils.VerifyGoLeaksOnce(t)
	})
	c := getESHttpClient(t)
	require.NoError(t, healthCheck(c))
	s := &ESStorageIntegration{}
	s.initializeES(t, true)
	s.testSyncBulkWriter(t)
}

// TestElasticsearchStorage_WriteIdempotency proves the deterministic content-hash
// _id (RFC 0007 §4.7) makes span writes idempotent end-to-end: writing the same
// trace twice through the real trace writer yields exactly one document (an
// op_type: index upsert), not a duplicate. The op_type: create side of §4.7 (a 409
// treated as a benign idempotent write) is covered by the esclient bulk unit test
// and the live 409 in TestElasticsearchStorage_SyncBulkWriter; a full data-stream
// end-to-end test follows once data-stream rotation is wired.
func TestElasticsearchStorage_WriteIdempotency(t *testing.T) {
	SkipUnlessEnv(t, StorageElasticsearch, StorageOpenSearch)
	t.Cleanup(func() {
		testutils.VerifyGoLeaksOnce(t)
	})
	c := getESHttpClient(t)
	require.NoError(t, healthCheck(c))
	s := &ESStorageIntegration{}
	s.initializeES(t, false)
	s.testWriteIdempotency(t)
}

func (s *ESStorageIntegration) testWriteIdempotency(t *testing.T) {
	ctx := context.Background()
	tID := pcommon.TraceID([16]byte{0, 0, 0, 0, 0, 0, 0, 33, 0, 0, 0, 0, 0, 0, 0, 44})
	trace := ptrace.NewTraces()
	rs := trace.ResourceSpans().AppendEmpty()
	rs.Resource().Attributes().PutStr("service.name", "idempotent_service")
	span := rs.ScopeSpans().AppendEmpty().Spans().AppendEmpty()
	span.SetName("idempotent_span")
	span.SetTraceID(tID)
	span.SetSpanID([8]byte{0, 0, 0, 0, 0, 0, 0, 66})
	span.SetStartTimestamp(pcommon.NewTimestampFromTime(time.Now().Truncate(time.Microsecond)))
	span.SetEndTimestamp(span.StartTimestamp())

	// Write the identical trace twice. With a server-generated _id this would store
	// two span documents; the deterministic _id makes the second write upsert onto
	// the first, so exactly one document remains.
	require.NoError(t, s.TraceWriter.WriteTraces(ctx, trace))
	require.NoError(t, s.TraceWriter.WriteTraces(ctx, trace))

	var actual ptrace.Traces
	found := s.waitForCondition(t, func(_ *testing.T) bool {
		iterTraces := s.TraceReader.GetTraces(ctx, tracestore.GetTraceParams{TraceID: tID})
		traces, err := jiter.CollectWithErrors(jptrace.AggregateTraces(iterTraces))
		if err != nil || len(traces) == 0 {
			return false
		}
		actual = traces[0]
		return true
	})
	require.True(t, found, "the span should be durably readable")
	assert.Equal(t, 1, actual.SpanCount(), "writing the same span twice must yield exactly one document")
}

func (s *ESStorageIntegration) testSyncBulkWriter(t *testing.T) {
	ctx := context.Background()
	index := indexPrefix + "-syncbulk"
	// TODO(RFC 0007 M4): this drives the low-level SyncBulkWriter directly, unlike
	// the other integration tests that go through tracestore.Writer (which in the
	// e2e configuration may be a remote writer). Once synchronous mode is wired
	// into the ES trace writer, prefer exercising it through WriteTraces so the
	// whole write path — remote included — is covered.
	writer := esclient.NewSyncBulkWriter(s.client.client, 0, metrics.NullFactory, zap.NewNop())
	searcher := esclient.SearchClient{Client: s.client.client}
	// Scoped teardown: remove only the one-off index this test created. The suite's
	// esCleanUp / factory.Purge are both DeleteAllIndices ("*"), not a prefix-scoped
	// cleanup, so they are no narrower; this test writes a bespoke index with the raw
	// client (outside the factory's managed span/service indices), so it owns and
	// deletes exactly that index rather than wiping the whole cluster. Isolation from
	// other tests comes from their own setup wipe, not this teardown.
	t.Cleanup(func() {
		require.NoError(t, s.client.indices.DeleteIndices(context.Background(), []esclient.Index{{Index: index}}))
	})

	// One blocking _bulk indexes both documents; a nil return means durable.
	require.NoError(t, writer.WriteBatch(ctx, []esclient.BulkItem{
		{Index: index, ID: "sb-1", OpType: esstorage.WriteOpCreate, Body: map[string]any{"name": "one"}},
		{Index: index, ID: "sb-2", OpType: esstorage.WriteOpCreate, Body: map[string]any{"name": "two"}},
	}))

	// Durability round-trip: both documents are retrievable (poll for the ~1s
	// refresh to make them searchable — durability, not search visibility, is what
	// the write guaranteed).
	require.Eventually(t, func() bool {
		resp, err := searcher.Search(ctx, []string{index}, esclient.SearchRequest{Size: 10})
		return err == nil && len(resp.Hits.Hits) == 2
	}, 10*time.Second, 100*time.Millisecond, "both documents should be durably readable")

	// Item-level error propagation with a partial batch: one new document (sb-3)
	// succeeds while re-creating an existing _id (sb-1) is rejected with a 409
	// version conflict. The sync writer surfaces the rejection as a real error —
	// the whole point of RFC 0007 — even though the sibling item was written.
	err := writer.WriteBatch(ctx, []esclient.BulkItem{
		{Index: index, ID: "sb-3", OpType: esstorage.WriteOpCreate, Body: map[string]any{"name": "three"}},
		{Index: index, ID: "sb-1", OpType: esstorage.WriteOpCreate, Body: map[string]any{"name": "one"}},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "1 of 2 bulk items rejected")
	assert.Contains(t, err.Error(), "id=sb-1", "the rejected item's _id aids debugging")

	// The non-conflicting item was still durably written (now three documents).
	require.Eventually(t, func() bool {
		resp, err := searcher.Search(ctx, []string{index}, esclient.SearchRequest{Size: 10})
		return err == nil && len(resp.Hits.Hits) == 3
	}, 10*time.Second, 100*time.Millisecond, "the non-conflicting document should be durably written")
}

// testArchiveTrace validates that a trace with a start time older than maxSpanAge
// can still be retrieved via the archive storage. This ensures archived traces are
// accessible even when their age exceeds the retention period for primary storage.
// This test applies only to Elasticsearch (ES) storage.
func (s *ESStorageIntegration) testArchiveTrace(t *testing.T) {
	s.skipIfNeeded(t)
	defer s.cleanUp(t)
	tID := pcommon.TraceID([16]byte{0, 0, 0, 0, 0, 0, 0, 11, 0, 0, 0, 0, 0, 0, 0, 22})
	expected := ptrace.NewTraces()
	rs := expected.ResourceSpans().AppendEmpty()
	rs.Resource().Attributes().PutStr("service.name", "archived_service")
	ss := rs.ScopeSpans().AppendEmpty()
	span := ss.Spans().AppendEmpty()
	span.SetName("archive_span")
	span.SetTraceID(tID)
	span.SetSpanID([8]byte{0, 0, 0, 0, 0, 0, 0, 55})
	span.SetStartTimestamp(pcommon.NewTimestampFromTime(time.Now().Add(-maxSpanAge * 5).Truncate(time.Microsecond)))
	span.SetEndTimestamp(span.StartTimestamp())
	require.NoError(t, s.ArchiveTraceWriter.WriteTraces(context.Background(), expected))

	var actual ptrace.Traces
	found := s.waitForCondition(t, func(_ *testing.T) bool {
		iterTraces := s.ArchiveTraceReader.GetTraces(context.Background(), tracestore.GetTraceParams{TraceID: tID})
		traces, err := jiter.CollectWithErrors(jptrace.AggregateTraces(iterTraces))
		if err != nil {
			t.Logf("Error loading trace: %v", err)
			return false
		}
		if len(traces) == 0 {
			return false
		}
		actual = traces[0]
		return actual.SpanCount() >= expected.SpanCount()
	})
	require.True(t, found)
	CompareTraces(t, expected, actual)
}
