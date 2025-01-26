// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	elasticsearch8 "github.com/elastic/go-elasticsearch/v8"
	"github.com/olivere/elastic"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"

	"github.com/jaegertracing/jaeger/pkg/config"
	"github.com/jaegertracing/jaeger/pkg/metrics"
	"github.com/jaegertracing/jaeger/pkg/testutils"
	"github.com/jaegertracing/jaeger/plugin/storage/es"
	"github.com/jaegertracing/jaeger/storage/dependencystore"
	"github.com/jaegertracing/jaeger/storage_v2/v1adapter"
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
	primaryNamespace         = "es"
	archiveNamespace         = "es-archive"
)

type ESStorageIntegration struct {
	StorageIntegration

	client   *elastic.Client
	v8Client *elasticsearch8.Client

	factory        *es.Factory
	archiveFactory *es.Factory
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

func (*ESStorageIntegration) initializeESFactory(t *testing.T, args []string, factoryInit func() *es.Factory) *es.Factory {
	logger := zaptest.NewLogger(t, zaptest.WrapOptions(zap.AddCaller()))
	f := factoryInit()
	v, command := config.Viperize(f.AddFlags)
	require.NoError(t, command.ParseFlags(args))
	f.InitFromViper(v, logger)
	require.NoError(t, f.Initialize(metrics.NullFactory, logger))

	t.Cleanup(func() {
		require.NoError(t, f.Close())
	})
	return f
}

func (s *ESStorageIntegration) initSpanstore(t *testing.T, allTagsAsFields bool) {
	f := s.initializeESFactory(t, []string{
		fmt.Sprintf("--es.num-shards=%v", 5),
		fmt.Sprintf("--es.num-replicas=%v", 1),
		fmt.Sprintf("--es.index-prefix=%v", indexPrefix),
		fmt.Sprintf("--es.use-ilm=%v", false),
		fmt.Sprintf("--es.service-cache-ttl=%v", 1*time.Second),
		fmt.Sprintf("--es.tags-as-fields.all=%v", allTagsAsFields),
		fmt.Sprintf("--es.bulk.actions=%v", 1),
		fmt.Sprintf("--es.bulk.flush-interval=%v", time.Nanosecond),
	}, es.NewFactory)
	af := s.initializeESFactory(t, []string{
		"--es-archive.enabled=true",
		fmt.Sprintf("--es-archive.tags-as-fields.all=%v", allTagsAsFields),
		fmt.Sprintf("--es-archive.index-prefix=%v", indexPrefix),
	}, es.NewArchiveFactory)
	s.factory = f
	s.archiveFactory = af
	var err error
	spanWriter, err := f.CreateSpanWriter()
	require.NoError(t, err)
	s.TraceWriter = v1adapter.NewTraceWriter(spanWriter)
	spanReader, err := f.CreateSpanReader()
	require.NoError(t, err)
	s.TraceReader = v1adapter.NewTraceReader(spanReader)
	s.ArchiveSpanReader, err = af.CreateSpanReader()
	require.NoError(t, err)
	s.ArchiveSpanWriter, err = af.CreateSpanWriter()
	require.NoError(t, err)

	dependencyReader, err := f.CreateDependencyReader()
	require.NoError(t, err)
	s.DependencyReader = v1adapter.NewDependencyReader(dependencyReader)

	s.DependencyWriter = v1adapter.NewDependencyWriter(dependencyReader.(dependencystore.Writer))

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
			Fixtures:        LoadAndParseQueryTestCases(t, "fixtures/queries_es.json"),
			SkipArchiveTest: false,
			// TODO: remove this flag after ES supports returning spanKind
			//  Issue https://github.com/jaegertracing/jaeger/issues/1923
			GetOperationsMissingSpanKind: true,
		},
	}
	s.initializeES(t, c, allTagsAsFields)
	s.RunAll(t)
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
