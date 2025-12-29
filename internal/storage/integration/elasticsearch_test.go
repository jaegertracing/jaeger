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
		if pingResult.Version.Number[0] == '1' || pingResult.Version.Number[0] == '2' || pingResult.Version.Number[0] == '3' {
			esVersion = 7
		}
	}
	return uint(esVersion), nil
}

func (s *ESStorageIntegration) initializeES(c *http.Client, allTagsAsFields bool) error {
	rawClient, err := elastic.NewClient(
		elastic.SetURL(queryURL),
		elastic.SetSniff(false),
		elastic.SetHttpClient(c),
	)
	if err != nil {
		return err
	}
	s.client = rawClient
	s.v8Client, err = elasticsearch8.NewClient(elasticsearch8.Config{
		Addresses:            []string{queryURL},
		DiscoverNodesOnStart: false,
		Transport:            c.Transport,
	})
	if err != nil {
		return err
	}

	if err := s.initSpanstore(allTagsAsFields); err != nil {
		return err
	}

	return s.esCleanUp()
}

func (s *ESStorageIntegration) esCleanUp() error {
	if err := s.factory.Purge(context.Background()); err != nil {
		return err
	}
	if err := s.archiveFactory.Purge(context.Background()); err != nil {
		return err
	}
	return nil
}

func (s *ESStorageIntegration) initSpanstore(allTagsAsFields bool) error {
	cfg := es.DefaultConfig()
	cfg.CreateIndexTemplates = true
	cfg.BulkProcessing = escfg.BulkProcessing{
		MaxActions:    1,
		FlushInterval: time.Nanosecond,
	}
	cfg.Tags.AllAsFields = allTagsAsFields
	cfg.ServiceCacheTTL = time.Second
	cfg.Indices.IndexPrefix = indexPrefix

	f, err := esv2.NewFactory(context.Background(), cfg, telemetry.NoopSettings(), nil)
	if err != nil {
		return err
	}
	acfg := es.DefaultConfig()
	acfg.ReadAliasSuffix = archiveAliasSuffix
	acfg.WriteAliasSuffix = archiveAliasSuffix
	acfg.UseReadWriteAliases = true
	acfg.Tags.AllAsFields = allTagsAsFields
	acfg.Indices.IndexPrefix = indexPrefix
	af, err := esv2.NewFactory(context.Background(), acfg, telemetry.NoopSettings(), nil)
	if err != nil {
		return err
	}
	s.factory = f
	s.archiveFactory = af
	if s.TraceWriter, err = f.CreateTraceWriter(); err != nil {
		return err
	}
	if s.TraceReader, err = f.CreateTraceReader(); err != nil {
		return err
	}
	if s.ArchiveTraceWriter, err = af.CreateTraceWriter(); err != nil {
		return err
	}
	if s.ArchiveTraceReader, err = af.CreateTraceReader(); err != nil {
		return err
	}
	if s.DependencyReader, err = f.CreateDependencyReader(); err != nil {
		return err
	}
	s.DependencyWriter = s.DependencyReader.(depstore.Writer)

	if s.SamplingStore, err = f.CreateSamplingStore(1); err != nil {
		return err
	}

	return nil
}

func (s *ESStorageIntegration) cleanESIndexTemplates(prefix string) error {
	version, err := s.getVersion()
	if err != nil {
		return err
	}

	if version > 7 {
		p := prefix
		if p != "" {
			p += "-"
		}
		if _, err = s.v8Client.Indices.DeleteIndexTemplate(p + spanTemplateName); err != nil {
			return err
		}
		if _, err = s.v8Client.Indices.DeleteIndexTemplate(p + serviceTemplateName); err != nil {
			return err
		}
		if _, err = s.v8Client.Indices.DeleteIndexTemplate(p + dependenciesTemplateName); err != nil {
			return err
		}
	} else {
		_, err = s.client.IndexDeleteTemplate("*").Do(context.Background())
		if err != nil {
			return err
		}
	}

	return nil
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
	require.NoError(t, s.initializeES(c, allTagsAsFields))
	s.CleanUp = func(t *testing.T) {
		require.NoError(t, s.esCleanUp())
		require.NoError(t, s.factory.Close())
		require.NoError(t, s.archiveFactory.Close())
		s.client.Stop()
	}
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
	require.NoError(t, s.initializeES(c, true))
	esVersion, err := s.getVersion()
	require.NoError(t, err)
	// TODO abstract this into pkg/es/client.IndexManagementLifecycleAPI
	if esVersion == 6 || esVersion == 7 {
		ok, err := s.client.IndexTemplateExists(indexPrefix + "-jaeger-service").Do(context.Background())
		require.NoError(t, err)
		assert.True(t, ok)
	} else {
		resp, err := s.v8Client.API.Indices.ExistsIndexTemplate(indexPrefix + "-jaeger-service")
		require.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)
	}
	require.NoError(t, s.cleanESIndexTemplates(indexPrefix))
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
		iter := s.ArchiveTraceReader.GetTraces(context.Background(), tracestore.GetTraceParams{TraceID: v1adapter.FromV1TraceID(tID)})
		traces, err := v1adapter.V1TracesFromSeq2(iter)
		if len(traces) == 0 {
			return false
		}
		actual = traces[0]
		return err == nil && len(actual.Spans) == 1
	})
	require.True(t, found)
	CompareTraces(t, expected, actual)
}
