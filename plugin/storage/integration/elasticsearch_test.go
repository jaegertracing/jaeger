// Copyright (c) 2019 The Jaeger Authors.
// Copyright (c) 2017 Uber Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package integration

import (
	"context"
	"errors"
	"net/http"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/olivere/elastic"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/model"
	eswrapper "github.com/jaegertracing/jaeger/pkg/es/wrapper"
	"github.com/jaegertracing/jaeger/pkg/testutils"
	"github.com/jaegertracing/jaeger/plugin/storage/es"
	"github.com/jaegertracing/jaeger/plugin/storage/es/dependencystore"
	"github.com/jaegertracing/jaeger/plugin/storage/es/spanstore"
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
)

type ESStorageIntegration struct {
	StorageIntegration

	client        *elastic.Client
	bulkProcessor *elastic.BulkProcessor
	logger        *zap.Logger
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
	return uint(esVersion), nil
}

func (s *ESStorageIntegration) initializeES(allTagsAsFields, archive bool) error {
	rawClient, err := elastic.NewClient(
		elastic.SetURL(queryURL),
		elastic.SetSniff(false))
	if err != nil {
		return err
	}
	s.logger, _ = testutils.NewLogger()

	s.client = rawClient
	s.initSpanstore(allTagsAsFields, archive)
	s.CleanUp = func() error {
		return s.esCleanUp(allTagsAsFields, archive)
	}
	s.Refresh = s.esRefresh
	s.esCleanUp(allTagsAsFields, archive)
	// TODO: remove this flag after ES support returning spanKind when get operations
	s.NotSupportSpanKindWithOperation = true
	return nil
}

func (s *ESStorageIntegration) esCleanUp(allTagsAsFields, archive bool) error {
	_, err := s.client.DeleteIndex("*").Do(context.Background())
	if err != nil {
		return err
	}
	return s.initSpanstore(allTagsAsFields, archive)
}

func (s *ESStorageIntegration) initSpanstore(allTagsAsFields, archive bool) error {
	bp, _ := s.client.BulkProcessor().BulkActions(1).FlushInterval(time.Nanosecond).Do(context.Background())
	s.bulkProcessor = bp
	esVersion, err := s.getVersion()
	if err != nil {
		return err
	}
	client := eswrapper.WrapESClient(s.client, bp, esVersion)
	spanMapping, serviceMapping := es.GetSpanServiceMappings(5, 1, client.GetVersion())
	w := spanstore.NewSpanWriter(
		spanstore.SpanWriterParams{
			Client:            client,
			Logger:            s.logger,
			MetricsFactory:    metrics.NullFactory,
			IndexPrefix:       indexPrefix,
			AllTagsAsFields:   allTagsAsFields,
			TagDotReplacement: tagKeyDeDotChar,
			Archive:           archive,
		})
	err = w.CreateTemplates(spanMapping, serviceMapping)
	if err != nil {
		return err
	}
	s.SpanWriter = w
	s.SpanReader = spanstore.NewSpanReader(spanstore.SpanReaderParams{
		Client:            client,
		Logger:            s.logger,
		MetricsFactory:    metrics.NullFactory,
		IndexPrefix:       indexPrefix,
		MaxSpanAge:        maxSpanAge,
		TagDotReplacement: tagKeyDeDotChar,
		Archive:           archive,
		MaxDocCount:       defaultMaxDocCount,
	})
	dependencyStore := dependencystore.NewDependencyStore(client, s.logger, indexPrefix, indexDateLayout, defaultMaxDocCount)
	depMapping := es.GetDependenciesMappings(5, 1, client.GetVersion())
	err = dependencyStore.CreateTemplates(depMapping)
	if err != nil {
		return err
	}
	s.DependencyReader = dependencyStore
	s.DependencyWriter = dependencyStore
	return nil
}

func (s *ESStorageIntegration) esRefresh() error {
	err := s.bulkProcessor.Flush()
	if err != nil {
		return err
	}
	_, err = s.client.Refresh().Do(context.Background())
	return err
}

func healthCheck() error {
	for i := 0; i < 200; i++ {
		if _, err := http.Get(queryURL); err == nil {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return errors.New("elastic search is not ready")
}

func testElasticsearchStorage(t *testing.T, allTagsAsFields, archive bool) {
	if os.Getenv("STORAGE") != "elasticsearch" {
		t.Skip("Integration test against ElasticSearch skipped; set STORAGE env var to elasticsearch to run this")
	}
	if err := healthCheck(); err != nil {
		t.Fatal(err)
	}
	s := &ESStorageIntegration{}
	require.NoError(t, s.initializeES(allTagsAsFields, archive))

	s.Fixtures = LoadAndParseQueryTestCases(t, "fixtures/queries_es.json")

	if archive {
		t.Run("ArchiveTrace", s.testArchiveTrace)
	} else {
		s.IntegrationTestAll(t)
	}
}

func TestElasticsearchStorage(t *testing.T) {
	testElasticsearchStorage(t, false, false)
}

func TestElasticsearchStorage_AllTagsAsObjectFields(t *testing.T) {
	testElasticsearchStorage(t, true, false)
}

func TestElasticsearchStorage_Archive(t *testing.T) {
	testElasticsearchStorage(t, false, true)
}

func (s *StorageIntegration) testArchiveTrace(t *testing.T) {
	defer s.cleanUp(t)
	tID := model.NewTraceID(uint64(11), uint64(22))
	expected := &model.Span{
		OperationName: "archive_span",
		StartTime:     time.Now().Add(-maxSpanAge * 5),
		TraceID:       tID,
		SpanID:        model.NewSpanID(55),
		References:    []model.SpanRef{},
		Process:       model.NewProcess("archived_service", model.KeyValues{}),
	}

	require.NoError(t, s.SpanWriter.WriteSpan(context.Background(), expected))
	s.refresh(t)

	var actual *model.Trace
	found := s.waitForCondition(t, func(t *testing.T) bool {
		var err error
		actual, err = s.SpanReader.GetTrace(context.Background(), tID)
		return err == nil && len(actual.Spans) == 1
	})
	if !assert.True(t, found) {
		CompareTraces(t, &model.Trace{Spans: []*model.Span{expected}}, actual)
	}
}
