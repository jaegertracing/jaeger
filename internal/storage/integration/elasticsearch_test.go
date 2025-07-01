// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	elasticsearch8 "github.com/elastic/go-elasticsearch/v9"
	"github.com/olivere/elastic/v7"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaegertracing/jaeger-idl/model/v1"
	escfg "github.com/jaegertracing/jaeger/internal/storage/elasticsearch/config"
	es "github.com/jaegertracing/jaeger/internal/storage/v1/elasticsearch"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/depstore"
	"github.com/jaegertracing/jaeger/internal/storage/v2/api/tracestore"
	esv2 "github.com/jaegertracing/jaeger/internal/storage/v2/elasticsearch"
	"github.com/jaegertracing/jaeger/internal/storage/v2/v1adapter"
	"github.com/jaegertracing/jaeger/internal/telemetry"
	"github.com/jaegertracing/jaeger/internal/testutils"
)

const (
	host                     = "0.0.0.0"
	queryPort                = "9200"
	queryHostPort            = host + ":" + queryPort
	queryURL                 = "http://" + queryHostPort
	indexPrefix              = "integration-test"
	indexDateLayout          = "2006-01-02"
	tagKeyDeDotChar          = "@"
	maxSpanAge               = time.Hour * 72
	defaultMaxDocCount       = 10_000
	spanTemplateName         = "jaeger-span"
	serviceTemplateName      = "jaeger-service"
	dependenciesTemplateName = "jaeger-dependencies"
	archiveAliasSuffix       = "archive"
)

type ESStorageIntegration struct {
	StorageIntegration

	client   *elastic.Client
	v8Client *elasticsearch8.Client

	ArchiveTraceReader tracestore.Reader
	ArchiveTraceWriter tracestore.Writer

	factory        *esv2.Factory
	archiveFactory *esv2.Factory
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
		if pingResult.Version.Number[0] == '1' || pingResult.Version.Number[0] == '2' {
			esVersion = 7
		}
	}
	return uint(esVersion), nil
}

func (s *ESStorageIntegration) initializeES(t *testing.T, c *http.Client, allTagsAsFields bool) {
	rawClient, err := elastic.NewClient(
		elastic.SetURL(queryURL),
		elastic.SetSniff(false),
		elastic.SetHttpClient(c))
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

func getESIndexOptions() escfg.IndexOptions {
	replicas := int64(1)
	return escfg.IndexOptions{
		Shards:            5,
		Replicas:          &replicas,
		DateLayout:        "2006-01-02",
		RolloverFrequency: "day",
	}
}

func getESIndices() escfg.Indices {
	return escfg.Indices{
		IndexPrefix:  indexPrefix,
		Spans:        getESIndexOptions(),
		Sampling:     getESIndexOptions(),
		Services:     getESIndexOptions(),
		Dependencies: getESIndexOptions(),
	}
}

func (s *ESStorageIntegration) initSpanstore(t *testing.T, allTagsAsFields bool) {
	cfg := escfg.Configuration{
		Servers:         []string{"http://127.0.0.1:9200"},
		UseILM:          false,
		ServiceCacheTTL: 1 * time.Second,
		Tags: escfg.TagsAsFields{
			AllAsFields: allTagsAsFields,
		},
		BulkProcessing: escfg.BulkProcessing{
			MaxActions:    1,
			FlushInterval: time.Nanosecond,
		},
		Indices:              getESIndices(),
		CreateIndexTemplates: true,
	}
	defaultCfg := es.DefaultConfig()
	cfg.ApplyDefaults(&defaultCfg)
	var err error
	f, err := esv2.NewFactory(context.Background(), cfg, telemetry.NoopSettings())
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, f.Close())
	})
	acfg := escfg.Configuration{
		Servers: []string{"http://127.0.0.1:9200"},
		Tags: escfg.TagsAsFields{
			AllAsFields: allTagsAsFields,
		},
		Indices:             getESIndices(),
		ReadAliasSuffix:     archiveAliasSuffix,
		WriteAliasSuffix:    archiveAliasSuffix,
		UseReadWriteAliases: true,
	}
	acfg.ApplyDefaults(&defaultCfg)
	af, err := esv2.NewFactory(context.Background(), acfg, telemetry.NoopSettings())
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
	for i := 0; i < 200; i++ {
		if resp, err := c.Get(queryURL); err == nil {
			return resp.Body.Close()
		}
		time.Sleep(100 * time.Millisecond)
	}
	return errors.New("elastic search is not ready")
}

func testElasticsearchStorage(t *testing.T, allTagsAsFields bool) {
	SkipUnlessEnv(t, "elasticsearch", "opensearch")
	c := getESHttpClient(t)
	require.NoError(t, healthCheck(c))
	s := &ESStorageIntegration{
		StorageIntegration: StorageIntegration{
			Fixtures: LoadAndParseQueryTestCases(t, "fixtures/queries_es.json"),
			// TODO: remove this flag after ES supports returning spanKind
			//  Issue https://github.com/jaegertracing/jaeger/issues/1923
			GetOperationsMissingSpanKind: true,
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
	testElasticsearchStorage(t, false)
}

func TestElasticsearchStorage_AllTagsAsObjectFields(t *testing.T) {
	t.Cleanup(func() {
		testutils.VerifyGoLeaksOnceForES(t)
	})
	testElasticsearchStorage(t, true)
}

func TestElasticsearchStorage_IndexTemplates(t *testing.T) {
	SkipUnlessEnv(t, "elasticsearch", "opensearch")
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

func (s *ESStorageIntegration) cleanESIndexTemplates(t *testing.T, prefix string) error {
	version, err := s.getVersion()
	require.NoError(t, err)
	if version > 7 {
		prefixWithSeparator := prefix
		if prefix != "" {
			prefixWithSeparator += "-"
		}
		_, err := s.v8Client.Indices.DeleteIndexTemplate(prefixWithSeparator + spanTemplateName)
		require.NoError(t, err)
		_, err = s.v8Client.Indices.DeleteIndexTemplate(prefixWithSeparator + serviceTemplateName)
		require.NoError(t, err)
		_, err = s.v8Client.Indices.DeleteIndexTemplate(prefixWithSeparator + dependenciesTemplateName)
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
	tID := model.NewTraceID(uint64(11), uint64(22))
	expected := &model.Trace{
		Spans: []*model.Span{
			{
				OperationName: "archive_span",
				StartTime:     time.Now().Add(-maxSpanAge * 5).Truncate(time.Microsecond),
				TraceID:       tID,
				SpanID:        model.NewSpanID(55),
				References:    []model.SpanRef{},
				Process:       model.NewProcess("archived_service", model.KeyValues{}),
			},
		},
	}

	require.NoError(t, s.ArchiveTraceWriter.WriteTraces(context.Background(), v1adapter.V1TraceToOtelTrace(expected)))

	var actual *model.Trace
	found := s.waitForCondition(t, func(_ *testing.T) bool {
		var err error
		iterTraces := s.ArchiveTraceReader.GetTraces(context.Background(), tracestore.GetTraceParams{TraceID: v1adapter.FromV1TraceID(tID)})
		traces, err := v1adapter.V1TracesFromSeq2(iterTraces)
		if len(traces) == 0 {
			return false
		}
		actual = traces[0]
		return err == nil && len(actual.Spans) == 1
	})
	require.True(t, found)
	CompareTraces(t, expected, actual)
}
