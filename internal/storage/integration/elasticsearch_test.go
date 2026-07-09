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
		MaxActions:    1,
		FlushInterval: time.Nanosecond,
	}
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

func runElasticsearchTest(t *testing.T, allTagsAsFields bool) {
	SkipUnlessEnv(t, StorageElasticsearch, StorageOpenSearch)
	c := getESHttpClient(t)
	require.NoError(t, healthCheck(c))
	s := &ESStorageIntegration{
		StorageIntegration: StorageIntegration{
			Fixtures:     LoadAndParseQueryTestCases(t, "fixtures/queries_es.json"),
			Capabilities: capabilities.Elasticsearch(),
		},
	}
	s.initializeES(t, allTagsAsFields)
	s.RunAll(t)
	t.Run("ArchiveTrace", s.testArchiveTrace)
}

func TestElasticsearchStorage(t *testing.T) {
	t.Cleanup(func() {
		testutils.VerifyGoLeaksOnce(t)
	})
	runElasticsearchTest(t, false)
}

func TestElasticsearchStorage_AllTagsAsObjectFields(t *testing.T) {
	t.Cleanup(func() {
		testutils.VerifyGoLeaksOnce(t)
	})
	runElasticsearchTest(t, true)
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

func (s *ESStorageIntegration) testSyncBulkWriter(t *testing.T) {
	ctx := context.Background()
	index := indexPrefix + "-syncbulk"
	writer := esclient.NewSyncBulkWriter(s.client.client, 0, metrics.NullFactory, zap.NewNop())
	searcher := esclient.SearchClient{Client: s.client.client}
	t.Cleanup(func() { s.client.deleteAllIndices(t) })

	// One blocking _bulk indexes both documents; a nil return means durable.
	require.NoError(t, writer.Bulk(ctx, []esclient.BulkItem{
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

	// Item-level error propagation: re-creating an existing _id with op_type=create
	// makes the backend reject the item (409 version conflict), and the sync writer
	// surfaces it as a real error — the whole point of RFC 0007.
	err := writer.Bulk(ctx, []esclient.BulkItem{
		{Index: index, ID: "sb-1", OpType: esstorage.WriteOpCreate, Body: map[string]any{"name": "one"}},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "rejected")
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
