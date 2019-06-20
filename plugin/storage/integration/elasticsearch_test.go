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
	"github.com/jaegertracing/jaeger/model"
	"github.com/stretchr/testify/assert"
	"net/http"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	"github.com/uber/jaeger-lib/metrics"
	"go.uber.org/zap"
	"gopkg.in/olivere/elastic.v5"

	"github.com/jaegertracing/jaeger/pkg/es/wrapper"
	"github.com/jaegertracing/jaeger/pkg/testutils"
	"github.com/jaegertracing/jaeger/plugin/storage/es"
	"github.com/jaegertracing/jaeger/plugin/storage/es/dependencystore"
	"github.com/jaegertracing/jaeger/plugin/storage/es/spanstore"
)

const (
	host            = "0.0.0.0"
	queryPort       = "9200"
	queryHostPort   = host + ":" + queryPort
	queryURL        = "http://" + queryHostPort
	username        = "elastic"  // the elasticsearch default username
	password        = "changeme" // the elasticsearch default password
	indexPrefix     = "integration-test"
	tagKeyDeDotChar = "@"
)

type ESStorageIntegration struct {
	StorageIntegration

	client        *elastic.Client
	bulkProcessor *elastic.BulkProcessor
	logger        *zap.Logger
}

func (s *ESStorageIntegration) initializeES(allTagsAsFields, archive bool) error {
	rawClient, err := elastic.NewClient(
		elastic.SetURL(queryURL),
		elastic.SetBasicAuth(username, password),
		elastic.SetSniff(false))
	if err != nil {
		return err
	}
	s.logger, _ = testutils.NewLogger()

	s.client = rawClient

	s.bulkProcessor, _ = s.client.BulkProcessor().Do(context.Background())
	client := eswrapper.WrapESClient(s.client, s.bulkProcessor)
	dependencyStore := dependencystore.NewDependencyStore(client, s.logger, indexPrefix)
	s.DependencyReader = dependencyStore
	s.DependencyWriter = dependencyStore
	s.initSpanstore(allTagsAsFields, archive)
	s.CleanUp = func() error {
		return s.esCleanUp(allTagsAsFields, archive)
	}
	s.Refresh = s.esRefresh
	s.esCleanUp(allTagsAsFields, archive)
	return nil
}

func (s *ESStorageIntegration) esCleanUp(allTagsAsFields, archive bool) error {
	_, err := s.client.DeleteIndex("*").Do(context.Background())
	s.initSpanstore(allTagsAsFields, archive)
	return err
}

func (s *ESStorageIntegration) initSpanstore(allTagsAsFields, archive bool) {
	bp, _ := s.client.BulkProcessor().BulkActions(1).FlushInterval(time.Nanosecond).Do(context.Background())
	client := eswrapper.WrapESClient(s.client, bp)
	spanMapping, serviceMapping := es.GetMappings(5, 1)
	s.SpanWriter = spanstore.NewSpanWriter(
		spanstore.SpanWriterParams{
			Client:            client,
			Logger:            s.logger,
			MetricsFactory:    metrics.NullFactory,
			IndexPrefix:       indexPrefix,
			AllTagsAsFields:   allTagsAsFields,
			TagDotReplacement: tagKeyDeDotChar,
			SpanMapping:         spanMapping,
			ServiceMapping:      serviceMapping,
			Archive: archive,
		})
	s.SpanReader = spanstore.NewSpanReader(spanstore.SpanReaderParams{
		Client:            client,
		Logger:            s.logger,
		MetricsFactory:    metrics.NullFactory,
		IndexPrefix:       indexPrefix,
		MaxSpanAge:        72 * time.Hour,
		TagDotReplacement: tagKeyDeDotChar,
		Archive: archive,
	})
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
	//if os.Getenv("STORAGE") != "elasticsearch" {
	//	t.Skip("Integration test against ElasticSearch skipped; set STORAGE env var to elasticsearch to run this")
	//}
	if err := healthCheck(); err != nil {
		t.Fatal(err)
	}
	s := &ESStorageIntegration{}
	//require.NoError(t, s.initializeES(allTagsAsFields, archive))
	//s.IntegrationTestAll(t)

	if archive {
		require.NoError(t, s.initializeES(allTagsAsFields, true))
		t.Run("ArchiveTrace", s.testArchiveTrace)
	}
}

//func TestElasticsearchStorage(t *testing.T) {
//	testElasticsearchStorage(t, false, false)
//}
//
//func TestElasticsearchStorage_AllTagsAsObjectFields(t *testing.T) {
//	testElasticsearchStorage(t, true, false)
//}

func TestElasticsearchStorage_Archive(t *testing.T) {
	testElasticsearchStorage(t, false, true)
}

func (s *StorageIntegration) testArchiveTrace(t *testing.T) {
	//defer s.cleanUp(t)
	tId := model.NewTraceID(uint64(22), uint64(44))
	//expected := s.loadParseAndWriteExampleTrace(t)
	//expectedTraceID := expected.Spans[0].TraceID
	//for i := 0; i < len(expected.Spans); i++ {
	//	expected.Spans[i].StartTime = time.Now().Add(-time.Hour*24*150)
	//	require.NoError(t, s.SpanWriter.WriteSpan(expected.Spans[i]))
	//}
	expected := &model.Span{
		OperationName:    "archive_span",
		StartTime: time.Now(),
		//StartTime: time.Now().Add(- time.Hour*24*5),

		TraceID: tId,
		SpanID: model.NewSpanID(1111),
		Process: model.NewProcess("archived_service", model.KeyValues{}),
	}

	//var actual *model.Trace
	//found := s.waitForCondition(t, func(t *testing.T) bool {
	//	var err error
	//	actual, err = s.SpanReader.GetTrace(context.Background(), expectedTraceID)
	//	if err != nil {
	//		t.Log(err)
	//	}
	//	return err == nil && len(actual.Spans) == len(expected.Spans)
	//})
	//if !assert.True(t, found) {
	//	CompareTraces(t, expected, actual)
	//}

	s.refresh(t)
	actual, err := s.SpanReader.GetTrace(context.Background(), tId)
	require.NoError(t, err)
	assert.EqualValues(t, expected, actual)
}
