// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	elasticsearch8 "github.com/elastic/go-elasticsearch/v9"
	"github.com/olivere/elastic/v7"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/config/configoptional"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/jaegertracing/jaeger/internal/jiter"
	"github.com/jaegertracing/jaeger/internal/jptrace"
	escfg "github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
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

	client   *elastic.Client
	v8Client *elasticsearch8.Client

	ArchiveTraceReader tracestore.Reader
	ArchiveTraceWriter tracestore.Writer

	factory        *esv2.Factory
	archiveFactory *esv2.Factory

	// spanRotation, when set, overrides the default periodic rotation for the
	// spans index (e.g. to exercise the data_stream strategy). nil keeps the
	// default periodic behavior used by the other tests.
	spanRotation *escfg.RotationConfig
}

func (s *ESStorageIntegration) getVersion() (uint, error) {
	pingResult, _, err := s.client.Ping(queryURL).Do(context.Background())
	if err != nil {
		return 0, err
	}
	esVersion, err := strconv.Atoi(string(pingResult.Version.Number[0]))
	if err != nil {
		return 0, err
	}
	// OpenSearch is based on ES 7.x
	if strings.Contains(pingResult.TagLine, "OpenSearch") {
		if pingResult.Version.Number[0] == '1' || pingResult.Version.Number[0] == '2' || pingResult.Version.Number[0] == '3' {
			esVersion = 7
		}
	}
	return uint(esVersion), nil
}

func (s *ESStorageIntegration) initializeES(t *testing.T, c *http.Client, allTagsAsFields bool) {
	rawClient, err := elastic.NewClient(
		elastic.SetURL(queryURL),
		elastic.SetSniff(false),
		elastic.SetHttpClient(c),
	)
	require.NoError(t, err)
	t.Cleanup(func() {
		rawClient.Stop()
	})
	s.client = rawClient
	s.v8Client, err = elasticsearch8.NewClient(elasticsearch8.Config{
		Addresses:            []string{queryURL},
		DiscoverNodesOnStart: false,
		Transport:            c.Transport,
	})
	require.NoError(t, err)

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
	if s.spanRotation != nil {
		cfg.Indices.Spans.Rotation = *s.spanRotation
	}
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
	s.initializeES(t, c, allTagsAsFields)
	s.RunAll(t)
	t.Run("ArchiveTrace", s.testArchiveTrace)
}

func TestElasticsearchStorage(t *testing.T) {
	t.Cleanup(func() {
		testutils.VerifyGoLeaksOnceForES(t)
	})
	runElasticsearchTest(t, false)
}

func TestElasticsearchStorage_AllTagsAsObjectFields(t *testing.T) {
	t.Cleanup(func() {
		testutils.VerifyGoLeaksOnceForES(t)
	})
	runElasticsearchTest(t, true)
}

func TestElasticsearchStorage_IndexTemplates(t *testing.T) {
	SkipUnlessEnv(t, StorageElasticsearch, StorageOpenSearch)
	t.Cleanup(func() {
		testutils.VerifyGoLeaksOnceForES(t)
	})
	c := getESHttpClient(t)
	require.NoError(t, healthCheck(c))
	s := &ESStorageIntegration{}
	s.initializeES(t, c, true)
	esVersion, err := s.getVersion()
	require.NoError(t, err)
	// TODO abstract this into pkg/es/client.IndexManagementLifecycleAPI
	if esVersion == 6 || esVersion == 7 {
		serviceTemplateExists, err := s.client.IndexTemplateExists(indexPrefix + "-jaeger-service").Do(context.Background())
		require.NoError(t, err)
		assert.True(t, serviceTemplateExists)
		spanTemplateExists, err := s.client.IndexTemplateExists(indexPrefix + "-jaeger-span").Do(context.Background())
		require.NoError(t, err)
		assert.True(t, spanTemplateExists)
	} else {
		serviceTemplateExistsResponse, err := s.v8Client.API.Indices.ExistsIndexTemplate(indexPrefix + "-jaeger-service")
		require.NoError(t, err)
		assert.Equal(t, 200, serviceTemplateExistsResponse.StatusCode)
		spanTemplateExistsResponse, err := s.v8Client.API.Indices.ExistsIndexTemplate(indexPrefix + "-jaeger-span")
		require.NoError(t, err)
		assert.Equal(t, 200, spanTemplateExistsResponse.StatusCode)
	}
	s.cleanESIndexTemplates(t, indexPrefix)
}

// TestElasticsearchStorage_DataStreams runs the full storage suite with the spans
// index configured to use the data_stream rotation strategy (RFC 0004 Phase 2).
// Writes go to the data stream via op_type=create and reads target the data stream
// name directly, exercising the end-to-end path on both Elasticsearch and
// OpenSearch. Services/dependencies/sampling keep the default periodic rotation.
func TestElasticsearchStorage_DataStreams(t *testing.T) {
	SkipUnlessEnv(t, "elasticsearch", "opensearch")
	t.Cleanup(func() {
		testutils.VerifyGoLeaksOnceForES(t)
	})
	c := getESHttpClient(t)
	require.NoError(t, healthCheck(c))
	// Data streams require Elasticsearch 7.9+ or OpenSearch 2.0+ (RFC 0004 §3.8);
	// older backends in the CI matrix (ES 6.x, OpenSearch 1.x) are skipped.
	skipUnlessDataStreamsSupported(t, c)
	dataStreamName := indexPrefix + ".jaeger.spans"
	// The shared Purge (DeleteIndex "*") cannot remove a data stream or its hidden
	// backing indices, so the test deletes its own data stream artifacts.
	t.Cleanup(func() { cleanESDataStream(t, c, dataStreamName) })

	s := &ESStorageIntegration{
		StorageIntegration: StorageIntegration{
			Fixtures:     LoadAndParseQueryTestCases(t, "fixtures/queries_es.json"),
			Capabilities: capabilities.Elasticsearch(),
		},
		spanRotation: &escfg.RotationConfig{
			DataStream: configoptional.Some(escfg.DataStreamRotation{}),
		},
	}
	s.initializeES(t, c, false)
	// Data streams only affect spans; services/dependencies/sampling stay periodic,
	// so only the span read/write path is exercised here (RFC 0004 Phase 2 item 14).
	s.RunSpanStoreTests(t)

	// After writing spans, the data stream must exist with at least one backing
	// index, proving spans were routed to the data stream rather than a dated index.
	requireDataStreamExists(t, c, dataStreamName)
}

// skipUnlessDataStreamsSupported skips the test on backends that predate data
// stream support, since the bootstrap (composable templates with data_stream:{})
// would otherwise fail on ES 6.x or OpenSearch 1.x.
func skipUnlessDataStreamsSupported(t *testing.T, c *http.Client) {
	resp, err := c.Get(queryURL)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	var info struct {
		Version struct {
			Number string `json:"number"`
		} `json:"version"`
		TagLine string `json:"tagline"`
	}
	require.NoError(t, json.Unmarshal(body, &info))
	parts := strings.Split(info.Version.Number, ".")
	major, _ := strconv.Atoi(parts[0])
	minor := 0
	if len(parts) > 1 {
		minor, _ = strconv.Atoi(parts[1])
	}
	if strings.Contains(info.TagLine, "OpenSearch") {
		if major < 2 {
			t.Skipf("data streams require OpenSearch 2.0+, got %s", info.Version.Number)
		}
		return
	}
	if major < 7 || (major == 7 && minor < 9) {
		t.Skipf("data streams require Elasticsearch 7.9+, got %s", info.Version.Number)
	}
}

func requireDataStreamExists(t *testing.T, c *http.Client, name string) {
	resp, err := c.Get(queryURL + "/_data_stream/" + name)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode, "data stream %q should exist: %s", name, body)
	assert.Contains(t, string(body), ".ds-"+name, "data stream should have a backing index")
}

func cleanESDataStream(t *testing.T, c *http.Client, name string) {
	del := func(path string) {
		req, err := http.NewRequestWithContext(context.Background(), http.MethodDelete, queryURL+path, http.NoBody)
		require.NoError(t, err)
		resp, err := c.Do(req)
		if err == nil {
			resp.Body.Close()
		}
	}
	// Order matters: the data stream references the index template, which composes
	// the component templates. The lifecycle policy is deleted last.
	del("/_data_stream/" + name)
	del("/_index_template/" + name)
	del("/_component_template/" + name + "@mappings")
	del("/_component_template/" + name + "@settings")
	del("/_plugins/_ism/policies/jaeger-spans-policy") // OpenSearch (ISM)
	del("/_ilm/policy/jaeger-spans-policy")            // Elasticsearch (ILM)
}

func (s *ESStorageIntegration) cleanESIndexTemplates(t *testing.T, prefix string) error {
	version, err := s.getVersion()
	require.NoError(t, err)
	if version > 7 {
		prefixWithSeparator := prefix
		if prefix != "" {
			prefixWithSeparator += "-"
		}
		_, err := s.v8Client.Indices.DeleteIndexTemplate([]string{prefixWithSeparator + escfg.SpanIndexName})
		require.NoError(t, err)
		_, err = s.v8Client.Indices.DeleteIndexTemplate([]string{prefixWithSeparator + escfg.ServiceIndexName})
		require.NoError(t, err)
		_, err = s.v8Client.Indices.DeleteIndexTemplate([]string{prefixWithSeparator + escfg.DependencyIndexName})
		require.NoError(t, err)
	} else {
		_, err := s.client.IndexDeleteTemplate("*").Do(context.Background())
		require.NoError(t, err)
	}
	return nil
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
